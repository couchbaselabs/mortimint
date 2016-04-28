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
	"math"
	"os"
	"path"
	"strings"
)

var ScannerBufferCapacity = 20 * 1024 * 1024

func main() {
	parseArgsToRun(os.Args).process()
}

// ------------------------------------------------------------

// Run is the main data struct that describes a processing run.
type Run struct {
	DictPath  string   // Path to optional dictionary file to output.
	EmitOrig  bool     // When true, also emit original log entries to stdout.
	EmitParts string   // Comma-separated list of parts of data to emit (NAME, STRS, TAIL).
	EmitTypes string   // Comma-separated list of value types to emit (INT, STRING),
	Dirs      []string // Directories to process.

	Web     bool   // When true, run a web server instead of emitting to stdout.
	WebBind string // Host:Port that web server should use.

	emitParts map[string]bool // True when that part should be emitted.
	emitTypes map[string]bool // True when that value type should be emitted.

	dict Dict
}

func parseArgsToRun(args []string) *Run {
	run := &Run{
		emitParts: map[string]bool{},
		emitTypes: map[string]bool{},
		dict:      Dict{},
	}

	flagSet := flag.NewFlagSet(args[0], flag.ExitOnError)

	flagSet.StringVar(&run.DictPath, "dictPath", "",
		"optional, path to output JSON dictionary file.")
	flagSet.BoolVar(&run.EmitOrig, "emitOrig", false,
		"when true, the original log lines are also emitted to stdout.")
	flagSet.StringVar(&run.EmitParts, "emitParts", "NAME",
		"optional, comma-separated list of parts to emit; valid values:\n"+
			"          NAME - emit name=value pairs;\n"+
			"          STRS - emit strings between name=value pairs;\n"+
			"          TAIL - emit string after last name=value pair;\n"+
			"       ")
	flagSet.StringVar(&run.EmitTypes, "emitTypes", "INT",
		"optional, comma-separated list of value types to emit; valid values:\n"+
			"          INT    - emit integer values;\n"+
			"          STRING - emit string values;\n"+
			"       ")
	flagSet.BoolVar(&run.Web, "web", false,
		"optional, when true, run a web server instead of emitting to stdout.")
	flagSet.StringVar(&run.WebBind, "webAddr", ":8911",
		"optional, addr:port that web server should use to bind/listen to.\n"+
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
	for _, dir := range run.Dirs {
		err := run.processDir(dir)
		if err != nil {
			log.Fatal(err)
		}
	}

	if run.DictPath != "" {
		fmt.Fprintf(os.Stderr, "emitting dictionary: %s\n", run.DictPath)

		for _, d := range run.dict {
			if d.MinInt >= math.MaxInt64 {
				d.MinInt = 0
			}
			if d.MaxInt <= math.MinInt64 {
				d.MaxInt = 0
			}
		}

		f, err := os.OpenFile(run.DictPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
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

func (run *Run) processDir(dir string) error {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	dirBase := path.Base(dir)

	for _, fileInfo := range fileInfos {
		fname := fileInfo.Name()

		fmeta, exists := FileMetas[fname]
		if !exists || fmeta.Skip {
			fmt.Fprintf(os.Stderr, "processing %s/%s, skipped\n", dirBase, fname)
			continue
		}

		fp := &fileProcessor{
			run:     run,
			dir:     dir,
			dirBase: dirBase,
			fname:   fname,
			fmeta:   fmeta,
			dict:    Dict{},
		}

		err := fp.process()
		if err != nil {
			return err
		}
	}

	return nil
}

// ------------------------------------------------------------

func (run *Run) emit(timeStamp, dirBase, fname string, startOffset, startLine int,
	partKind string, namePath []string, name, valType, val string, valQuoted bool) {
	if run.emitParts[partKind] && len(val) > 0 {
		if name != "" {
			name = name + " "
		}

		if valQuoted {
			fmt.Printf("  %s %s/%s:%d:%d %s %+v %s= %s %q\n",
				timeStamp, dirBase, fname, startOffset, startLine,
				partKind, namePath, name, valType, val)
		} else {
			fmt.Printf("  %s %s/%s:%d:%d %s %+v %s= %s %s\n",
				timeStamp, dirBase, fname, startOffset, startLine,
				partKind, namePath, name, valType, val)
		}
	}
}
