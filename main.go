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

	if run.run["web"] {
		run.web()
	} else if run.run["stdout"] {
		run.process(os.Stdout)
	} else {
		log.Fatalf("unknown kind of run: %s", run.Run)
	}
}

// ------------------------------------------------------------

// Run is the main data struct that describes a processing run.
type Run struct {
	EmitDict  string // Path to optional JSON dictionary file to output.
	EmitOrig  string // When non-"", original log entries will be emitted to stdout.
	EmitParts string // Comma-separated list of parts of data to emit (NAME, MIDS, ENDS).
	EmitTypes string // Comma-separated list of value types to emit (INT, STRING),

	Dirs []string // Directories to process.

	ProgressEvery int

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

	emitWriter io.Writer

	dict Dict

	m sync.Mutex

	progress int
}

func parseArgsToRun(args []string) (*Run, *flag.FlagSet) {
	run := &Run{
		fileProcessors: map[string]map[string]*fileProcessor{},
		dict:           Dict{},
	}

	flagSet := flag.NewFlagSet(args[0], flag.ExitOnError)

	flagSet.StringVar(&run.EmitDict, "emitDict", "",
		"optional, path to JSON dictionary output file.")
	flagSet.StringVar(&run.EmitOrig, "emitOrig", "",
		"when not the empty string (\"\"), source log lines are emitted to stdout;\n"+
			"        when \"1line\", source log lines are joined into a single line.")
	flagSet.StringVar(&run.EmitParts, "emitParts", "FULL",
		"optional, comma-separated list of parts to emit; supported values:\n"+
			"          FULL - emit full entry, with only light parsing;\n"+
			"          NAME - emit name=value pairs;\n"+
			"          MIDS - emit strings in between the name=value pairs;\n"+
			"          ENDS - emit string after last name=value pair.\n"+
			"       ")
	flagSet.StringVar(&run.EmitTypes, "emitTypes", "INT",
		"optional, comma-separated list of value types to emit; supported values:\n"+
			"          INT    - emit integer name=value pairs;\n"+
			"          STRING - emit string name=value pairs.\n"+
			"       ")
	flagSet.IntVar(&run.ProgressEvery, "progressEvery", 0,
		"optional, when > 0, emit a progress dot to stderr after modulo this many emits.")
	flagSet.StringVar(&run.Run, "run", "stdout",
		"optional, comma-separated list of the kind of run; supported values:\n"+
			"          stdout - emit processed logs to stdout;\n"+
			"          web    - run a webserver.\n"+
			"       ")
	flagSet.StringVar(&run.Tmp, "tmp", "",
		"optional, tmp dir to use, otherwise use a system auto-created tmp dir.")
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

func (run *Run) process(emitWriter io.Writer) {
	run.emitWriter = emitWriter

	numFiles := 0
	maxFNameOutLen := 0

	for _, dir := range run.Dirs {
		fileInfos, err := ioutil.ReadDir(dir)
		if err != nil {
			log.Fatal(err)
		}

		for _, fileInfo := range fileInfos {
			fmeta, exists := FileMetas[fileInfo.Name()]
			if exists && !fmeta.Skip {
				numFiles += 1

				x := len(path.Base(dir)) + len(fileInfo.Name()) + 1
				if maxFNameOutLen < x {
					maxFNameOutLen = x
				}
			}
		}
	}

	workCh := make(chan *fileProcessor, numFiles)
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
		err := run.processDir(dir, workCh, maxFNameOutLen)
		if err != nil {
			log.Fatal(err)
		}
	}

	close(workCh)

	for i := 0; i < numFiles; i++ {
		fp := <-doneCh
		fp.dict.AddTo(run.dict)
	}

	// -----------------------------------------------

	if run.EmitDict != "" {
		fmt.Fprintf(os.Stderr, "emitting JSON dictionary: %s\n", run.EmitDict)

		f, err := os.OpenFile(run.EmitDict, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
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

func (run *Run) processDir(dir string, workCh chan *fileProcessor,
	maxFNameOutLen int) error {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	dirBase := path.Base(dir)

	run.fileProcessors[dirBase] = map[string]*fileProcessor{}

	spaces := strings.Repeat(" ", maxFNameOutLen)

	for _, fileInfo := range fileInfos {
		fname := fileInfo.Name()
		fnameBaseParts := strings.Split(strings.Replace(fname, ".log", "", -1), ",")
		fnameBase := fnameBaseParts[len(fnameBaseParts)-1]

		fmeta, exists := FileMetas[fname]
		if !exists || fmeta.Skip {
			fmt.Fprintf(os.Stderr, "processing %s/%s, skipped\n", dirBase, fname)
			continue
		}

		run.fileProcessors[dirBase][fname] = &fileProcessor{
			run:       run,
			dir:       dir,
			dirBase:   dirBase,
			fname:     fname,
			fnameBase: fnameBase,
			fnameOut:  (dirBase + "/" + fname + spaces)[0:maxFNameOutLen],
			fmeta:     fmeta,
			dict:      Dict{},
		}

		workCh <- run.fileProcessors[dirBase][fname]
	}

	return nil
}

// ------------------------------------------------------------

func (run *Run) emitEntryFull(ts, module, level, dirBase,
	fname, fnameBase, fnameOut string, startOffset, startLine int, lines []string) {
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
	run.emitProgressLocked()
	run.m.Unlock()
}

func (run *Run) emitEntryPart(ts, module, level, dirBase,
	fname, fnameBase, fnameOut string, startOffset, startLine int,
	partKind string, namePath []string, name, valType, val string, valQuoted bool) {
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

		run.emitProgressLocked()

		run.m.Unlock()
	}
}

func (run *Run) emitProgressLocked() {
	run.progress++
	if run.ProgressEvery > 0 && (run.progress % run.ProgressEvery) == 0 {
		fmt.Fprint(os.Stderr, ".")
	}
}

func emitPrepCommon(module, fnameBase string, startOffset, startLine int) (string, string) {
	if module == "" {
		module = fnameBase
	}

	ol := fmt.Sprintf("%d:%d", startOffset, startLine)
	ol = (ol + "                ")[0:12]

	return module, ol
}
