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

	"go/scanner"
	"go/token"
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
		log.Printf("processFile, dir: %s, fname: %s, skipped, no file meta", dir, fname)
		return nil
	}

	if fileMeta.Skip {
		log.Printf("processFile, dir: %s, fname: %s, skipped", dir, fname)
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
	var buf []byte

	for scanner.Scan() {
		lineNum++
		if lineNum < fileMeta.FirstLine {
			continue
		}

		lineStr := scanner.Text()
		if fileMeta.EntryStart == nil ||
			fileMeta.EntryStart(lineStr) {
			buf = processEntry(dir, fname, entryStart, entryLines, buf)

			entryStart = lineNum
			entryLines = entryLines[0:0]
		}

		entryLines = append(entryLines, lineStr)
	}

	buf = processEntry(dir, fname, entryStart, entryLines, buf)

	return scanner.Err()
}

func processEntry(dir, fname string, startLine int, entryLines []string, buf []byte) []byte {
	if startLine <= 0 || len(entryLines) <= 0 {
		return buf
	}

	fmt.Printf("************* (%s => %s:%d)\n", dir, fname, startLine)

	nbytes := 0
	for _, entryLine := range entryLines {
		nbytes += len(entryLine)

		fmt.Println(entryLine)
	}

	buf = buf[0:0]
	for _, entryLine := range entryLines {
		buf = append(buf, []byte(entryLine)...)
		buf = append(buf, '\n')
	}

	// Hack to use go's scanner rather write our own.
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(buf)) // Fake file for buf.
	s.Init(file, buf, nil /* No error handler. */, 0)

	emitTokens(&s)

	return buf
}

var spaces = "                                             "+
	"                                                      "+
	"                                                      "+
	"                                                      "+
	"                                                      "+
	"                                                      "

var levelDelta = map[token.Token]int {
    token.LPAREN: 1,
    token.RPAREN: -1, // )
    token.LBRACK: 1,
    token.RBRACK: -1, // ]
    token.LBRACE: 1,
    token.RBRACE: -1, // }
}

func emitTokens(s *scanner.Scanner) {
	level := 0

	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		level += levelDelta[tok]
		if level < 0 {
			level = 0
		}

		if lit != "" {
			fmt.Printf("%s%s %s\n", spaces[0:level], tok, lit)
		} else {
			fmt.Printf("%s%s\n", spaces[0:level], tok)
		}
	}
}
