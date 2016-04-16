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
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
)

var ScannerBufferCapacity = 20*1024*1024

// ------------------------------------------------------------

func main() {
	err := processDirs(os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
}

func processDirs(dirs []string) error {
	for _, dir := range dirs {
		err := processDir(dir)
		if err != nil {
			return err
		}
	}

	return nil
}

func processDir(dir string) error {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, fileInfo := range fileInfos {
		fname := fileInfo.Name()
		if WantSuffixes[path.Ext(fname)] {
			err := processFile(dir, fname)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func processFile(dir, fname string) error {
	log.Printf("processFile, dir: %s, fname: %s", dir, fname)

	fileMeta, exists := FileMetas[fname]
	if !exists {
		return nil
	}

	log.Printf("processFile, dir: %s, fname: %s, opening", dir, fname)

	f, err := os.Open(dir + "/" + fname)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, ScannerBufferCapacity)

	var lineNum int
	var entryStart int
	var entryLines []string

	for scanner.Scan() {
		lineNum++
		if lineNum < fileMeta.LineFirst {
			continue
		}

		lineStr := scanner.Text()
		if fileMeta.EntryStart == nil ||
			fileMeta.EntryStart(lineStr) {
			processEntry(dir, fname, entryStart, entryLines)

			entryStart = lineNum
			entryLines = entryLines[0:0]
		}

		entryLines = append(entryLines, lineStr)
	}

	processEntry(dir, fname, entryStart, entryLines)

	return scanner.Err()
}

func processEntry(dir, fname string, startLine int, lines []string) {
	if startLine > 0 {
		fmt.Printf("************* (%s => %s:%d)\n", dir, fname, startLine)
		for _, entryLine := range lines {
			fmt.Println(entryLine)
		}
	}
}
