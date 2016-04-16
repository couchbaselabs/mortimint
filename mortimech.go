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
	"strings"
	"unicode"
)

var ScannerBufferCapacity = 20*1024*1024

func main() {
	processDirs(os.Args[1:])
}

func processDirs(dirs []string) {
	for _, dir := range dirs {
		err := processDir(dir)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func processDir(dir string) error {
	fileInfos, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, fileInfo := range fileInfos {
		if strings.HasSuffix(fileInfo.Name(), ".log") {
			err := processFile(dir, fileInfo.Name())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// ------------------------------------------------------------

type FileMeta struct {
	LineFirst  int
	LineSample string
	EntryStart func(string) bool
}

var FileMetas = map[string]FileMeta{ // Keyed by file name.
	"memcached.log": {
		LineFirst:  5,
		LineSample: "2016-04-14T16:10:09.463447-07:00 WARNING Restarting file logging",
	},
	"ns_server.babysitter.log": {
		LineFirst:  5,
		LineSample: `[ns_server:debug,2016-04-14T16:10:05.262-07:00,babysitter_of_ns_1@127.0.0.1:<0.65.0>:restartable:start_child:98]Started child process <0.66.0>
  MFA: {supervisor_cushion,start_link,
                           [ns_server,5000,infinity,ns_port_server,start_link,
                            [#Fun<ns_child_ports_sup.2.49698737>]]}`,
		EntryStart: func(line string) bool {
			if len(line) <= 0 ||
				line[0] != '[' {
				return false
			}
			lineParts := strings.Split(line, ",")
			if len(lineParts) < 3 || len(lineParts[1]) <= 0 {
				return false
			}
			return unicode.IsDigit(rune(lineParts[1][0]))
		},
	},
}

// ------------------------------------------------------------

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
	var entryLines []string

	for scanner.Scan() {
		lineNum++
		if lineNum < fileMeta.LineFirst {
			continue
		}

		lineStr := scanner.Text()
		if fileMeta.EntryStart == nil ||
			fileMeta.EntryStart(lineStr) {

			fmt.Println("*************")
			for _, entryLine := range entryLines {
				fmt.Println(entryLine)
			}

			entryLines = entryLines[0:0]
		}

		entryLines = append(entryLines, lineStr)
	}

	fmt.Println("*************")
	for _, entryLine := range entryLines {
		fmt.Println(entryLine)
	}

	return scanner.Err()
}
