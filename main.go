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

var ScannerBufferCapacity = 20 * 1024 * 1024

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
		if !WantSuffixes[path.Ext(fname)] {
			continue
		}

		err := processFile(dir, fname)
		if err != nil {
			return err
		}
	}

	return nil
}

func processFile(dir, fname string) error {
	log.Printf("processFile, dir: %s, fname: %s", dir, fname)

	fmeta, exists := FileMetas[fname]
	if !exists {
		log.Printf("processFile, dir: %s, fname: %s, skipped, no file meta", dir, fname)
		return nil
	}

	if fmeta.Skip {
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
		if lineNum <= fmeta.HeaderSize {
			continue
		}

		lineStr := scanner.Text()
		if fmeta.EntryStart == nil ||
			fmeta.EntryStart(lineStr) {
			buf = processEntry(dir, fname, &fmeta, entryStart, entryLines, buf)

			entryStart = lineNum
			entryLines = entryLines[0:0]
		}

		entryLines = append(entryLines, lineStr)
	}

	buf = processEntry(dir, fname, &fmeta, entryStart, entryLines, buf)

	return scanner.Err()
}

func processEntry(dir, fname string, fmeta *FileMeta,
	startLine int, entryLines []string, buf []byte) []byte {
	if startLine <= 0 || len(entryLines) <= 0 {
		return buf
	}

	for _, entryLine := range entryLines {
		fmt.Println(entryLine)
	}

	firstLine := entryLines[0]

	match := fmeta.PrefixRE.FindStringSubmatch(firstLine)
	if len(match) <= 0 {
		return buf
	}

	matchParts := map[string]string{}
	for i, name := range fmeta.PrefixRE.SubexpNames() {
		if i > 0 {
			matchParts[name] = match[i]
		}
	}

	entryLines[0] = firstLine[len(match[0]):]

	ts := string(fmeta.PrefixRE.ExpandString(nil,
		"${year}-${month}-${day}T${HH}:${MM}:${SS}-${SSSS}",
		firstLine,
		fmeta.PrefixRE.FindSubmatchIndex([]byte(firstLine))))

	buf = buf[0:0]
	buf = append(buf, fmeta.Prefix...)
	for _, entryLine := range entryLines {
		buf = append(buf, []byte(entryLine)...)
		buf = append(buf, '\n')
	}

	if fmeta.Cleanser != nil {
		buf = fmeta.Cleanser(buf)
	}

	// Hack to use go's tokenizer / scanner rather then write our own.
	var s scanner.Scanner
	fset := token.NewFileSet()
	file := fset.AddFile("", fset.Base(), len(buf)) // Fake file for buf.
	s.Init(file, buf, nil /* No error handler. */, 0)

	fmt.Println(ts)

	emitTokens(&s)

	return buf
}

var levelDelta = map[token.Token]int{
	token.LPAREN: 1,
	token.RPAREN: -1, // )
	token.LBRACK: 1,
	token.RBRACK: -1, // ]
	token.LBRACE: 1,
	token.RBRACE: -1, // }

	token.CHAR:   0,
	token.INT:    0,
	token.FLOAT:  0,
	token.STRING: 0,

	token.COLON:     0,
	token.COMMA:     0,
	token.PERIOD:    0,
	token.SEMICOLON: 0,
}

var skipToken = map[token.Token]bool{
	token.SHL: true, // <<
	token.SHR: true, // >>
}

func emitTokens(s *scanner.Scanner) {
	level := 0

	var prevLevel int
	var prevTok token.Token
	var prevLit string

	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		if skipToken[tok] {
			continue
		}

		delta, deltaExists := levelDelta[tok]
		if !deltaExists && prevTok != token.ILLEGAL {
			_, prevDeltaExists := levelDelta[prevTok]
			if !prevDeltaExists {
				prevLit =
					tokenLitString(prevTok, prevLit) + " " +
						tokenLitString(tok, lit)

				continue
			}
		}

		level += delta
		if level < 0 {
			level = 0
		}

		emitToken(prevLevel, prevTok, prevLit)

		prevLevel, prevTok, prevLit = level, tok, lit
	}

	emitToken(prevLevel, prevTok, prevLit)
}

func tokenLitString(tok token.Token, lit string) string {
	if lit != "" {
		return lit
	}

	return tok.String()
}

var spaces = "                                             " +
	"                                                      " +
	"                                                      " +
	"                                                      "

func emitToken(level int, tok token.Token, lit string) {
	if tok != token.ILLEGAL {
		if lit != "" {
			fmt.Printf("%s%s %s\n", spaces[0:level], tok, lit)
		} else {
			fmt.Printf("%s%s\n", spaces[0:level], tok)
		}
	}
}
