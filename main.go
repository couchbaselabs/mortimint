//  Copyright (c) 2016 Couchbase, Inc.
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the
//  License. You may obtain a copy of the License at
//    http://www.apache.org/licenses/LICENSE-2.0
//  Unless required by applicable law or agreed to in writing,
//  software distributed under the License is distributed on an "AS
//  IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
//  express or implied. See the License for the specific language
//  governing permissions and limitations under the License.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
)

var ScannerBufferCapacity = 20 * 1024 * 1024

func main() {
	run, flagSet := parseArgsToRun(os.Args)

	fmt.Fprintf(os.Stderr, "%s\n", os.Args[0])

	flagSet.VisitAll(func(f *flag.Flag) {
		fmt.Fprintf(os.Stderr, "  -%s=%s\n", f.Name, f.Value)
	})

	if run.run["stdout"] {
		run.process(os.Stdout)
	}

	var processDoneCh chan struct{}

	if run.run["tmp"] || run.run["web"] {
		var tmpDirForCleanup string
		tmpDirForCleanup, processDoneCh = run.processTmp()
		if tmpDirForCleanup != "" {
			defer os.RemoveAll(tmpDirForCleanup)
		}
	}

	if run.run["webServer"] || run.run["web"] {
		go run.webServer()

		if processDoneCh != nil {
			<-processDoneCh
		}

		fmt.Fprintf(os.Stderr, "\nmortimint web (ctrl-d to exit) >> ")

		ioutil.ReadAll(os.Stdin)
	}
}

// ------------------------------------------------------------

func (run *Run) processTmp() (tmpDirForCleanup string, doneCh chan struct{}) {
	if run.Tmp == "" {
		tmp, err := ioutil.TempDir("", "mortimint.tmp.")
		if err != nil {
			log.Fatal(err)
		}
		run.Tmp = tmp

		tmpDirForCleanup = tmp
	}

	if run.EmitDict == "" {
		run.EmitDict = run.Tmp + string(os.PathSeparator) + "emit.dict"
	}

	emitLogPath := run.Tmp + string(os.PathSeparator) + "emit.log"
	emitLogFile, err := os.OpenFile(emitLogPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Fprintf(os.Stderr, "emitting: emit.dict: %s\n", run.EmitDict)
	fmt.Fprintf(os.Stderr, "emitting: emit.log: %s\n", emitLogPath)

	if run.ProgressEvery == 0 {
		run.ProgressEvery = 10000
	}

	doneCh = make(chan struct{})

	go func() {
		defer close(doneCh)

		run.process(emitLogFile)

		emitLogFile.Close()

		fmt.Fprintf(os.Stderr, "\ndone: emited files...\n  %s\n  %s\n",
			run.EmitDict, emitLogPath)

		fmt.Fprintf(os.Stderr, "\nexamples:\n\n  grep curr_items %s\n",
			emitLogPath)
	}()

	return tmpDirForCleanup, doneCh
}

// ------------------------------------------------------------

// Run is the main data struct that describes a processing run.
type Run struct {
	EmitDict  string // Path to optional JSON dictionary file to output.
	EmitOrig  string // When non-"", original log entries will be emitted to stdout.
	EmitParts string // Comma-separated list of parts of data to emit (NAME, MIDS, ENDS).
	EmitTypes string // Comma-separated list of value types to emit (INT, STRING),

	Dirs []string // Directories to process.

	ProgressEvery int // When > 0 emit progress every this many entries.

	Run string // Comma-separated list of the kind of run, like "stdout,web".
	Tmp string // Tmp dir to use, otherwise use a system provided tmp dir.

	WebBind   string // Host:Port that web server should use.
	WebStatic string // Path to web static resources dir.

	Workers int // Size of workers pool for concurrency.

	emitParts map[string]bool // True when that part should be emitted.
	emitTypes map[string]bool // True when that value type should be emitted.
	run       map[string]bool

	// fileProcessors is keyed by dirBase, then by file name.
	fileProcessors map[string]map[string]*fileProcessor
	fileSizes      map[string]map[string]int64

	numFiles       int
	maxFNameOutLen int
	spaces         string // len(spaces) == maxFNameOutLen, used for padding.

	m sync.Mutex // Protects the fields that follow.

	emitWriter io.Writer

	dict Dict

	emitDone     bool
	emitProgress int64                       // Total number of emitXxxx() calls.
	fileProgress map[string]map[string]int64 // Byte offsets reached.
}

func parseArgsToRun(args []string) (*Run, *flag.FlagSet) {
	run := &Run{
		fileProcessors: map[string]map[string]*fileProcessor{},
		fileSizes:      map[string]map[string]int64{},
		fileProgress:   map[string]map[string]int64{},
		dict:           Dict{},
	}

	flagSet := flag.NewFlagSet(args[0], flag.ExitOnError)

	flagSet.StringVar(&run.EmitDict, "emitDict", "",
		"optional, path to JSON dictionary output file.")
	flagSet.StringVar(&run.EmitOrig, "emitOrig", "",
		"when not the empty string (\"\"), source log lines are emitted to stdout;\n"+
			"        when \"single\", source log entries are joined into a single line.")
	flagSet.StringVar(&run.EmitParts, "emitParts", "FULL",
		"optional, comma-separated list of parts to emit; supported values:\n"+
			"          FULL - emit full log entry, with only light parsing;\n"+
			"          NAME - emit name=value pairs;\n"+
			"          MIDS - emit strings in between the name=value pairs;\n"+
			"          ENDS - emit string after last name=value pair.\n"+
			"       ")
	flagSet.StringVar(&run.EmitTypes, "emitTypes", "INT",
		"optional, comma-separated list of NAME value types to emit; supported values:\n"+
			"          INT    - emit integer name=value pairs;\n"+
			"          STRING - emit string name=value pairs.\n"+
			"       ")
	flagSet.IntVar(&run.ProgressEvery, "progressEvery", 0,
		"optional, when > 0, emit a progress to stderr after modulo this many emits.")
	flagSet.StringVar(&run.Run, "run", "stdout",
		"optional, comma-separated list of the kind of run; supported values:\n"+
			"          stdout    - emit processed logs to stdout;\n"+
			"          tmp       - emit processed logs and dict to tmp dir;\n"+
			"          web       - convenience alias for \"tmp,webServer\";\n"+
			"          webServer - run a web server with previously processed logs and dict.\n"+
			"       ")
	flagSet.StringVar(&run.Tmp, "tmp", "",
		"optional, tmp dir to use; a tmp dir will be created when run kind has \"tmp\".")
	flagSet.StringVar(&run.WebBind, "webAddr", ":8911",
		"optional, addr:port when running a web server.\n"+
			"       ")
	flagSet.StringVar(&run.WebStatic, "webStatic", "./static",
		"optional, directory of web static resources.\n"+
			"       ")
	flagSet.IntVar(&run.Workers, "workers", 1,
		"optional, number of concurrent processing workers to use.\n"+
			"       ")

	flagSet.Parse(args[1:])

	run.Dirs = flagSet.Args()

	run.emitParts = csvToMap(run.EmitParts, map[string]bool{})
	run.emitTypes = csvToMap(run.EmitTypes, map[string]bool{})
	run.run = csvToMap(run.Run, map[string]bool{})

	return run, flagSet
}

func csvToMap(csv string, m map[string]bool) map[string]bool {
	for _, k := range strings.Split(csv, ",") {
		m[k] = true
	}
	return m
}

// ------------------------------------------------------------

func (run *Run) process(emitWriter io.Writer) bool {
	if !run.processInit() {
		return false // No files to process.
	}

	run.emitWriter = emitWriter

	workCh := make(chan *fileProcessor, run.numFiles)
	doneCh := make(chan *fileProcessor)

	for i := 0; i < run.Workers; i++ {
		go func() {
			for fp := range workCh {
				err := fp.process()
				if err != nil {
					log.Fatal(err)
				}
				doneCh <- fp
			}
		}()
	}

	for _, dir := range run.Dirs {
		err := run.processDir(dir, workCh)
		if err != nil {
			log.Fatal(err)
		}
	}

	close(workCh)

	for i := 0; i < run.numFiles; i++ {
		fp := <-doneCh
		run.m.Lock()
		fp.dict.AddTo(run.dict)
		run.fileProgress[fp.dirBase][fp.fname] = run.fileSizes[fp.dirBase][fp.fname]
		run.m.Unlock()
	}

	run.processEmitDict()

	run.m.Lock()
	run.emitDone = true
	if run.ProgressEvery > 0 {
		run.emitProgressBarsLocked()
	}
	run.m.Unlock()

	return true
}

func (run *Run) processInit() bool {
	for _, dir := range run.Dirs {
		fileInfos, err := ioutil.ReadDir(dir)
		if err != nil {
			log.Fatal(err)
		}

		dirBase := path.Base(dir)

		for _, fileInfo := range fileInfos {
			fmeta, exists := FileMetas[fileInfo.Name()]
			if exists && !fmeta.Skip {
				run.numFiles += 1

				x := len(dirBase) + len(fileInfo.Name()) + 1
				if run.maxFNameOutLen < x {
					run.maxFNameOutLen = x
				}

				if run.fileSizes[dirBase] == nil {
					run.fileSizes[dirBase] = map[string]int64{}
				}
				run.fileSizes[dirBase][fileInfo.Name()] = fileInfo.Size()
			}
		}
	}

	run.spaces = strings.Repeat(" ", run.maxFNameOutLen)

	return run.numFiles > 0
}

func (run *Run) processEmitDict() {
	if run.EmitDict != "" {
		fmt.Fprintf(os.Stderr, "emitting JSON dictionary: %s\n", run.EmitDict)

		f, err := os.OpenFile(run.EmitDict,
			os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		err = json.NewEncoder(f).Encode(run.dict)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func (run *Run) processDir(dir string, workCh chan *fileProcessor) error {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	dirBase := path.Base(dir)

	run.fileProcessors[dirBase] = map[string]*fileProcessor{}

	for _, fileInfo := range fileInfos {
		fname := fileInfo.Name()
		fnameBaseParts := strings.Split(strings.Replace(fname, ".log", "", -1), ",")
		fnameBase := fnameBaseParts[len(fnameBaseParts)-1]

		fmeta, exists := FileMetas[fname]
		if !exists || fmeta.Skip {
			continue
		}

		run.fileProcessors[dirBase][fname] = &fileProcessor{
			run:       run,
			dir:       dir,
			dirBase:   dirBase,
			fname:     fname,
			fnameBase: fnameBase,
			fnameOut:  (dirBase + "/" + fname + run.spaces)[0:run.maxFNameOutLen],
			fmeta:     fmeta,
			dict:      Dict{},
		}

		workCh <- run.fileProcessors[dirBase][fname]
	}

	return nil
}

// ------------------------------------------------------------

func (run *Run) emitEntryFull(ts, module, level, dirBase,
	fname, fnameBase, fnameOut string,
	startOffset, startLine int64, lines []string) {
	linesJoined := strings.Replace(strings.Join(lines, " "), "\n", " ", -1)

	module, ol := emitPrepCommon(module, fnameBase, startOffset, startLine)

	partKind := ""
	if len(run.emitParts) > 1 {
		partKind = "FULL "
	}

	run.m.Lock()
	fmt.Fprintf(run.emitWriter, "  %s %s %s %s %s%s ",
		ts, level, fnameOut, ol, partKind, module)
	fmt.Fprintln(run.emitWriter, linesJoined)
	run.emitProgressLocked(dirBase, fname, startOffset)
	run.m.Unlock()
}

func (run *Run) emitEntryPart(ts, module, level, dirBase,
	fname, fnameBase, fnameOut string,
	startOffset, startLine int64, partKind string,
	namePath []string, name, valType, val string, valQuoted bool) {
	if run.emitParts[partKind] && len(val) > 0 {
		module, ol := emitPrepCommon(module, fnameBase, startOffset, startLine)

		if len(run.emitParts) <= 1 {
			partKind = ""
		} else if partKind != "" {
			partKind = partKind + " "
		}

		if name != "" {
			name = name + " "
		}

		run.m.Lock()

		if valQuoted {
			fmt.Fprintf(run.emitWriter, "  %s %s %s %s %s%s %+v %s= %s %q\n",
				ts, level, fnameOut, ol, partKind, module,
				namePath, name, valType, val)
		} else {
			fmt.Fprintf(run.emitWriter, "  %s %s %s %s %s%s %+v %s= %s %s\n",
				ts, level, fnameOut, ol, partKind, module,
				namePath, name, valType, val)
		}

		run.emitProgressLocked(dirBase, fname, startOffset)

		run.m.Unlock()
	}
}

func emitPrepCommon(module, fnameBase string, startOffset, startLine int64) (
	string, string) {
	if module == "" {
		module = fnameBase
	}

	ol := fmt.Sprintf("%d:%d", startOffset, startLine)
	ol = (ol + "                ")[0:12]

	return module, ol
}

func (run *Run) emitProgressLocked(dirBase, fname string, offsetReached int64) {
	fileProgress := run.fileProgress[dirBase]
	if fileProgress == nil {
		fileProgress = map[string]int64{}
		run.fileProgress[dirBase] = fileProgress
	}
	fileProgress[fname] = offsetReached

	run.emitProgress++
	if run.ProgressEvery > 0 &&
		(run.emitProgress%int64(run.ProgressEvery)) == 0 {
		run.emitProgressBarsLocked()
	}
}

func (run *Run) emitProgressBarsLocked() {
	fmt.Fprintf(os.Stderr, "***\n")

	var dirBases []string
	for dirBase := range run.fileSizes {
		dirBases = append(dirBases, dirBase)
	}
	sort.Strings(dirBases)

	for _, dirBase := range dirBases {
		fileProgress := run.fileProgress[dirBase]

		fileSizes := run.fileSizes[dirBase]

		var fnames []string
		for fname := range fileSizes {
			fnames = append(fnames, fname)
		}
		sort.Strings(fnames)

		for _, fname := range fnames {
			fsize := fileSizes[fname]

			pct := 0.0
			if fileProgress != nil {
				pct = float64(fileProgress[fname]) / float64(fsize)
			}

			fnameOut := (dirBase + "/" + fname + run.spaces)[0:run.maxFNameOutLen]

			fmt.Fprintf(os.Stderr, "  %s %3d %s\n",
				fnameOut, int(pct*100.0), bars[0:int(pct*float64(len(bars)))])
		}
	}
}

var bars = "================================"
var spaces = "                                "


