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
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"sync"
)

var ScannerBufferCapacity = 20 * 1024 * 1024

func main() {
	parseArgsToRun(os.Args).process()
}

// ------------------------------------------------------------

// Run is the main data struct that describes a processing run.
type Run struct {
	DictJson  string   // Path to optional JSON dictionary file to output.
	EmitOrig  int      // When >0, also emit original log entries to stdout.
	EmitParts string   // Comma-separated list of parts of data to emit (NAME, MIDS, ENDS).
	EmitTypes string   // Comma-separated list of value types to emit (INT, STRING),
	Dirs      []string // Directories to process.

	Workers int // Size of workers pool for concurrency.

	emitParts map[string]bool // True when that part should be emitted.
	emitTypes map[string]bool // True when that value type should be emitted.

	// fileProcessors is keyed by dirBase, then by file name.
	fileProcessors map[string]map[string]*fileProcessor

	dict Dict

	m sync.Mutex
}

func parseArgsToRun(args []string) *Run {
	run := &Run{
		emitParts:      map[string]bool{},
		emitTypes:      map[string]bool{},
		fileProcessors: map[string]map[string]*fileProcessor{},
		dict:           Dict{},
	}

	flagSet := flag.NewFlagSet(args[0], flag.ExitOnError)

	flagSet.StringVar(&run.DictJson, "dictJson", "",
		"optional, path to JSON dictionary output file.")
	flagSet.IntVar(&run.EmitOrig, "emitOrig", 0,
		"when 1, original log lines are also emitted to stdout;\n"+
			"        when 2, original log lines are joined into a single line."+
			"       ")
	flagSet.StringVar(&run.EmitParts, "emitParts", "FULL",
		"optional, comma-separated list of parts to emit; valid values:\n"+
			"          FULL - emit full entry, with only light parsing;\n"+
			"          NAME - emit name=value pairs;\n"+
			"          MIDS - emit strings in between the name=value pairs;\n"+
			"          ENDS - emit string after last name=value pair.\n"+
			"       ")
	flagSet.StringVar(&run.EmitTypes, "emitTypes", "INT",
		"optional, comma-separated list of value types to emit; valid values:\n"+
			"          INT    - emit integer name=value pairs;\n"+
			"          STRING - emit string name=value pairs.\n"+
			"       ")
	flagSet.IntVar(&run.Workers, "workers", 1,
		"optional, number of concurrent workers to use.\n"+
			"       ")

	flagSet.Parse(args[1:])

	run.Dirs = flagSet.Args()

	for _, part := range strings.Split(run.EmitParts, ",") {
		run.emitParts[part] = true
	}

	for _, typE := range strings.Split(run.EmitTypes, ",") {
		run.emitTypes[typE] = true
	}

	return run
}

// ------------------------------------------------------------

func (run *Run) process() {
	maxFNameOutLen := 0
	for _, dir := range run.Dirs {
		fileInfos, err := ioutil.ReadDir(dir)
		if err != nil {
			log.Fatal(err)
		}

		for _, fileInfo := range fileInfos {
			fmeta, exists := FileMetas[fileInfo.Name()]
			if exists && !fmeta.Skip {
				x := len(path.Base(dir)) + len(fileInfo.Name()) + 1
				if maxFNameOutLen < x {
					maxFNameOutLen = x
				}
			}
		}
	}

	workCh := make(chan *fileProcessor, len(run.Dirs)*100)
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

	for _, fps := range run.fileProcessors {
		for range fps {
			fp := <-doneCh
			fp.dict.AddTo(run.dict)
		}
	}

	// -----------------------------------------------

	if run.DictJson != "" {
		fmt.Fprintf(os.Stderr, "emitting JSON dictionary: %s\n", run.DictJson)

		f, err := os.OpenFile(run.DictJson, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
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

		fmeta, exists := FileMetas[fname]
		if !exists || fmeta.Skip {
			fmt.Fprintf(os.Stderr, "processing %s/%s, skipped\n", dirBase, fname)
			continue
		}

		run.fileProcessors[dirBase][fname] = &fileProcessor{
			run:      run,
			dir:      dir,
			dirBase:  dirBase,
			fname:    fname,
			fnameOut: (dirBase + "/" + fname + spaces)[0:maxFNameOutLen],
			fmeta:    fmeta,
			dict:     Dict{},
		}

		workCh <- run.fileProcessors[dirBase][fname]
	}

	return nil
}

// ------------------------------------------------------------

func (run *Run) emitEntryFull(timeStamp, module, level, dirBase, fname, fnameOut string,
	startOffset, startLine int, lines []string) {
	linesJoined := strings.Replace(strings.Join(lines, " "), "\n", " ", -1)

	module, ol := emitPrepCommon(module, startOffset, startLine)

	run.m.Lock()
	fmt.Printf("  %s %s %s %s %s ", timeStamp, level, fnameOut, ol, module)
	fmt.Println(linesJoined)
	run.m.Unlock()
}

func (run *Run) emitEntryPart(timeStamp, module, level, dirBase, fname, fnameOut string,
	startOffset, startLine int, partKind string, namePath []string,
	name, valType, val string, valQuoted bool) {
	if run.emitParts[partKind] && len(val) > 0 {
		module, ol := emitPrepCommon(module, startOffset, startLine)

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
			fmt.Printf("  %s %s %s %s %s%s %+v %s= %s %q\n",
				timeStamp, level, fnameOut, ol,
				partKind, module, namePath, name, valType, val)
		} else {
			fmt.Printf("  %s %s %s %s %s%s %+v %s= %s %s\n",
				timeStamp, level, fnameOut, ol,
				partKind, module, namePath, name, valType, val)
		}

		run.m.Unlock()
	}
}

func emitPrepCommon(module string, startOffset, startLine int) (string, string) {
	if module == "" {
		module = "?"
	}

	ol := fmt.Sprintf("%d:%d", startOffset, startLine)
	ol = (ol + "                ")[0:12]

	return module, ol
}
