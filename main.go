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
	"runtime"
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

	emittedFiles := map[string]io.Closer{} // Keyed by path.

	if run.run["stdout"] || run.run["std"] {
		run.addEmitter(run.EmitParts, run.EmitTypes, os.Stdout)
	}

	if run.run["tmp"] || run.run["web"] {
		if run.OutDir == "" {
			tmp, err := ioutil.TempDir("", "mortimint.tmp.")
			if err != nil {
				log.Fatal(err)
			}
			run.OutDir = tmp

			defer os.RemoveAll(tmp)
		}
	}

	if run.run["emit"] || run.run["web"] {
		path, closer := run.addEmitterFile(run.OutDir, "full.log", "FULL", "")
		emittedFiles[path] = closer

		path, closer = run.addEmitterFile(run.OutDir, "vals.log", "VALS", "INT")
		emittedFiles[path] = closer

		if run.EmitParts != "FULL" || run.EmitTypes != "INT" {
			path, closer = run.addEmitterFile(run.OutDir, "emit.log", run.EmitParts, run.EmitTypes)
			emittedFiles[path] = closer
		}

		if run.EmitDict == "" {
			run.EmitDict = run.OutDir + string(os.PathSeparator) + "emit.dict"
		}

		if run.ProgressEvery == 0 {
			run.ProgressEvery = 10000
		}

		if run.Workers <= 0 {
			run.Workers = runtime.NumCPU()
		}
	}

	if run.run["webServer"] || run.run["web"] {
		go run.webServer()
	}

	if len(run.emitters) > 0 {
		run.processDirs()
	}

	if len(emittedFiles) > 0 {
		for _, f := range emittedFiles {
			f.Close()
		}

		fmt.Fprintf(os.Stderr, "\ndone, emitted directory and files:\n")
		fmt.Fprintf(os.Stderr, "  %s\n", run.OutDir)
		for path := range emittedFiles {
			fmt.Fprintf(os.Stderr, "  %s\n", path)
		}
	}

	if (run.run["stdin"] || run.run["std"]) && len(run.Dirs) <= 0 {
		run.webGraph(os.Stdin)
	}

	if run.run["webServer"] || run.run["web"] {
		webAddr := run.WebAddr
		if len(webAddr) > 0 && webAddr[0] == ':' {
			webAddr = "127.0.0.1" + webAddr
		}

		if run.WebStatic != "" {
			fmt.Fprintf(os.Stderr, "\nmortimint web static resources: %s\n", run.WebStatic)
		}

		fmt.Fprintf(os.Stderr, "\nmortimint web running on:\n  http://%s\n", webAddr)
		fmt.Fprintf(os.Stderr, "\nmortimint web (ctrl-d to exit) >> ")

		ioutil.ReadAll(os.Stdin)
	}
}

// ------------------------------------------------------------

// Run is the main data struct that describes a processing run.
type Run struct {
	EmitDict  string // Path to optional JSON dictionary file to output.
	EmitOrig  string // When non-"", original log entries will be emitted to stdout.
	EmitParts string // Comma-separated list of parts of data to emit (VALS, MIDS, ENDS).
	EmitTypes string // Comma-separated list of value types to emit (INT, STRING).

	Dirs []string // Input directories to process.

	OutDir string // Output directory to use.

	ProgressEvery int // When > 0 emit progress every this many entries.

	Run string // Comma-separated list of the kind of run, like "stdout,web".

	WebAddr   string // Host:Port to use for web server.
	WebStatic string // Path to web static resources dir.

	Workers int // Size of workers pool for concurrency.

	run map[string]bool // Result of parsing the Run param.

	totFiles       int // Total number of files to process.
	maxFNameOutLen int
	spaces         string // len(spaces) == maxFNameOutLen, used for padding.

	// fileSizes is keyed by dirBase, then by file name.
	fileSizes map[string]map[string]int64

	// fileProcessors is keyed by dirBase, then by file name.
	fileProcessors map[string]map[string]*fileProcessor

	emitters []*Emitter

	m sync.Mutex // Protects the fields that follow.

	emitDone     bool
	emitProgress int64                       // Total number of emitXxxx() calls.
	fileProgress map[string]map[string]int64 // Byte offsets reached.

	minTS, maxTS string

	dict Dict
}

// ------------------------------------------------------------

func parseArgsToRun(args []string) (*Run, *flag.FlagSet) {
	run := &Run{
		fileSizes:      map[string]map[string]int64{},
		fileProcessors: map[string]map[string]*fileProcessor{},
		fileProgress:   map[string]map[string]int64{},
		dict:           Dict{},
	}

	flagSet := flag.NewFlagSet(args[0], flag.ExitOnError)

	flagSet.StringVar(&run.EmitDict, "emitDict", "",
		"optional, path to JSON dictionary output file.")
	flagSet.StringVar(&run.EmitOrig, "emitOrig", "",
		"when not the empty string (\"\"), source log lines are emitted to stdout;\n"+
			"        when \"single\", source log entries are joined into a single line;\n"+
			"        this is useful when debugging mortimint.")
	flagSet.StringVar(&run.EmitParts, "emitParts", "FULL",
		"optional, comma-separated list of parts to emit; supported values:\n"+
			"          FULL - emit full log entry, with only light parsing;\n"+
			"          VALS - emit name=value pairs;\n"+
			"          MIDS - uncommon; emit strings in between the name=value pairs;\n"+
			"          ENDS - uncommon; emit string after last name=value pair.\n"+
			"       ")
	flagSet.StringVar(&run.EmitTypes, "emitTypes", "INT",
		"optional, comma-separated list of VALS value types to emit; supported values:\n"+
			"          INT    - emit integer name=value pairs;\n"+
			"          STRING - emit string name=value pairs.\n"+
			"       ")
	flagSet.IntVar(&run.ProgressEvery, "progressEvery", 0,
		"optional, when > 0, emit a progress to stderr after modulo this many emits.")
	flagSet.StringVar(&run.Run, "run", "std",
		"optional, comma-separated list of the kind of run; supported values:\n"+
			"          emit      - emits full/vals.log and emit.dict to outDir;\n"+
			"          std       - convenience alias for \"stdin,stdout\";\n"+
			"          stdin     - process stdin to send to web server for graphing;\n"+
			"          stdout    - emit processed logs to stdout;\n"+
			"          tmp       - create a temporary dir for outDir, if needed;\n"+
			"          web       - convenience alias for \"tmp,emit,webServer\";\n"+
			"          webServer - run a web server with previously emit'ed logs and dict.\n"+
			"       ")
	flagSet.StringVar(&run.OutDir, "outDir", "",
		"optional, output directory to use.")
	flagSet.StringVar(&run.WebAddr, "webAddr", ":8911",
		"optional, addr:port to use for web server.\n"+
			"       ")
	flagSet.StringVar(&run.WebStatic, "webStatic", "",
		"optional, directory of static web server resources;\n"+
			"        this is useful when debugging mortimint.")
	flagSet.IntVar(&run.Workers, "workers", 0,
		"optional, number of concurrent processing workers to use.\n"+
			"       ")

	flagSet.Parse(args[1:])

	run.Dirs = flagSet.Args()

	for _, dir := range run.Dirs {
		fileInfos, err := ioutil.ReadDir(dir)
		if err != nil {
			log.Fatal(err)
		}

		dirBase := path.Base(dir)

		for _, fileInfo := range fileInfos {
			fmeta, exists := FileMetas[fileInfo.Name()]
			if exists && !fmeta.Skip {
				run.totFiles += 1

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

	run.spaces = strings.Repeat(" ", run.maxFNameOutLen+1)

	run.run = csvToMap(run.Run, map[string]bool{})

	return run, flagSet
}

// ------------------------------------------------------------

func (run *Run) processDirs() bool {
	workCh := make(chan *fileProcessor, run.totFiles)
	doneCh := make(chan *fileProcessor)

	workers := run.Workers
	if workers <= 0 {
		workers = 1
	}

	for i := 0; i < workers; i++ {
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

	for i := 0; i < run.totFiles; i++ {
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

func (run *Run) processDir(dir string, workCh chan *fileProcessor) error {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	dirBase := path.Base(dir)

	run.fileProcessors[dirBase] = map[string]*fileProcessor{}

	for _, fileInfo := range fileInfos {
		fname := fileInfo.Name()
		fnameBaseParts := strings.Split(strings.Replace(fname, ".log", "", -1), ".")
		fnameBase := fnameBaseParts[len(fnameBaseParts)-1]

		fmeta, exists := FileMetas[fname]
		if !exists || fmeta.Skip {
			continue
		}

		run.m.Lock()
		run.fileProgress[dirBase] = map[string]int64{}
		run.m.Unlock()

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

func (run *Run) processEmitDict() {
	if run.EmitDict != "" {
		fmt.Fprintf(os.Stderr, "emitting JSON dictionary: %s\n", run.EmitDict)

		f, err := os.OpenFile(run.EmitDict, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		err = json.NewEncoder(f).Encode(struct {
			MinTS string
			MaxTS string
			Dict  Dict
		}{run.minTS, run.maxTS, run.dict})
		if err != nil {
			log.Fatal(err)
		}
	}
}

// ------------------------------------------------------------

func (run *Run) emitEntryFull(ts, module, level, dirBase,
	fname, fnameBase, fnameOut, ol string,
	startOffset, startLine int64, lines []string) {
	var linesJoined string

	run.m.Lock()

	for _, emitter := range run.emitters {
		if emitter.emitParts["FULL"] {
			if linesJoined == "" {
				linesJoined = strings.Replace(strings.Join(lines, " "), "\n", " ", -1)
			}

			emitter.emitEntryFull(ts, module, level, fnameOut, ol, linesJoined)
		}
	}

	run.emitCommonLocked(ts, dirBase, fname, startOffset)

	run.m.Unlock()
}

func (run *Run) emitEntryPart(ts, module, level, dirBase,
	fname, fnameBase, fnameOut, ol string,
	startOffset, startLine int64, partKind string,
	namePath []string, name, valType, val string, valQuoted bool) {
	if len(val) > 0 {
		run.m.Lock()

		for _, emitter := range run.emitters {
			emitter.emitEntryPart(ts, module, level,
				fnameOut, ol, partKind, namePath, name, valType, val, valQuoted)
		}

		run.emitCommonLocked(ts, dirBase, fname, startOffset)

		run.m.Unlock()
	}
}

func emitCommonPrep(module, fnameBase string, startOffset, startLine int64) (
	string, string) {
	if module == "" {
		module = fnameBase
	}

	ol := fmt.Sprintf("%d:%d", startOffset, startLine)
	ol = (ol + "                ")[0:12]

	return module, ol
}

func (run *Run) emitCommonLocked(ts, dirBase, fname string, offsetReached int64) {
	run.fileProgress[dirBase][fname] = offsetReached

	run.emitProgress++
	if run.ProgressEvery > 0 &&
		(run.emitProgress%int64(run.ProgressEvery)) == 0 {
		run.emitProgressBarsLocked()
	}

	if run.minTS == "" || run.minTS > ts {
		run.minTS = ts
	}

	if run.maxTS < ts {
		run.maxTS = ts
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
