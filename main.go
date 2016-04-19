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
	"strings"

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

		fp := &fileProcessor{dir: dir, fname: fname}

		err := fp.process()
		if err != nil {
			return err
		}
	}

	return nil
}

// ------------------------------------------------------------

type fileProcessor struct {
	dir   string
	fname string
	fmeta FileMeta
	buf   []byte
}

func (p *fileProcessor) process() error {
	fmt.Fprintf(os.Stderr, "processFile, dir: %s, fname: %s\n", p.dir, p.fname)

	fmeta, exists := FileMetas[p.fname]
	if !exists {
		fmt.Fprintf(os.Stderr,
			"processFile, dir: %s, fname: %s, skipped, no file meta\n", p.dir, p.fname)
		return nil
	}

	p.fmeta = fmeta
	if p.fmeta.Skip {
		fmt.Fprintf(os.Stderr,
			"processFile, dir: %s, fname: %s, skipped\n", p.dir, p.fname)
		return nil
	}

	fmt.Fprintf(os.Stderr,
		"processFile, dir: %s, fname: %s, opening\n", p.dir, p.fname)

	f, err := os.Open(p.dir + "/" + p.fname)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(nil, ScannerBufferCapacity)

	var currOffset int
	var currLine int
	var entryStartOffset int
	var entryStartLine int
	var entryLines []string

	for scanner.Scan() {
		lineStr := scanner.Text()

		currLine++
		if currLine <= p.fmeta.HeaderSize { // Skip header.
			currOffset += len(lineStr) + 1
			continue
		}

		if p.fmeta.EntryStart == nil ||
			p.fmeta.EntryStart(lineStr) {
			p.processEntry(entryStartOffset, entryStartLine, entryLines)

			entryStartOffset = currOffset
			entryStartLine = currLine
			entryLines = entryLines[0:0]
		}

		entryLines = append(entryLines, lineStr)
		currOffset += len(lineStr) + 1
	}

	p.processEntry(entryStartOffset, entryStartLine, entryLines)

	return scanner.Err()
}

func (p *fileProcessor) processEntry(startOffset, startLine int, lines []string) {
	if startLine <= 0 || len(lines) <= 0 {
		return
	}

	for _, line := range lines {
		fmt.Println(line)
		// _ = fmt.Println
		// _ = line
	}

	firstLine := lines[0]

	match := p.fmeta.PrefixRE.FindStringSubmatch(firstLine)
	if len(match) <= 0 {
		return
	}

	lines[0] = firstLine[len(match[0]):] // Strip off PrefixRE's match.

	matchParts := map[string]string{}
	for i, name := range p.fmeta.PrefixRE.SubexpNames() {
		if i > 0 {
			matchParts[name] = match[i]
		}
	}

	ts := string(p.fmeta.PrefixRE.ExpandString(nil,
		"${year}-${month}-${day}T${HH}:${MM}:${SS}-${SSSS}",
		firstLine,
		p.fmeta.PrefixRE.FindSubmatchIndex([]byte(firstLine))))

	p.buf = p.buf[0:0]
	for _, line := range lines {
		p.buf = append(p.buf, []byte(line)...)
		p.buf = append(p.buf, '\n')
	}

	if p.fmeta.Cleanser != nil {
		p.buf = p.fmeta.Cleanser(p.buf)
	}

	var s scanner.Scanner // Use go's tokenizer to parse entry.

	fset := token.NewFileSet()

	s.Init(fset.AddFile(p.dir+"/"+p.fname, fset.Base(), len(p.buf)),
		p.buf, nil /* No error handler. */, 0)

	p.processEntryScanner(ts, &s, nil)
}

var levelDelta = map[token.Token]int{
	token.LPAREN: 1,
	token.RPAREN: -1, // )
	token.LBRACK: 1,
	token.RBRACK: -1, // ]
	token.LBRACE: 1,
	token.RBRACE: -1, // }

	// When value is 0, it means don't change level, and also don't
	// merge into neighboring tokens.

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

// A tokLit associates a token and a literal string.
type tokLit struct {
	level int
	tok   token.Token
	lit   string
}

func (p *fileProcessor) processEntryScanner(ts string,
	s *scanner.Scanner, path []string) {
	level := len(path)

	tokLits := make([]tokLit, 4) // Track some previous tokLit's.

	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		if skipToken[tok] {
			continue
		}

		// If the token doesn't have a level delta, and is merge'able
		// with the previous token, then merge.  For example, this
		// merges an IDENT that's followed by an IDENT.
		delta, deltaExists := levelDelta[tok]
		if !deltaExists && tokLits[0].tok != token.ILLEGAL {
			_, prevDeltaExists := levelDelta[tokLits[0].tok]
			if !prevDeltaExists {
				tokLits[0].lit =
					tokenLitString(tokLits[0].tok, tokLits[0].lit) + " " +
						tokenLitString(tok, lit)

				continue
			}
		}

		tokLits[3] = tokLits[2]
		tokLits[2] = tokLits[1]
		tokLits[1] = tokLits[0]
		tokLits[0] = tokLit{level, tok, lit}

		p.processEntryTokLits(ts, path, tokLits)

		if delta > 0 {
			pathSub := path
			pathPart := nameFromTokLits(tokLits, 1)
			if pathPart != "" {
				pathSub = append(pathSub, pathPart)
			}

			p.processEntryScanner(ts, s, pathSub) // Recurse.
		} else if delta < 0 {
			return
		}
	}

	if p.processEntryTokLits(ts, path, tokLits) == "" {
		suffix := nameFromTokLits(tokLits, 0)
		if suffix != "" {
			fmt.Printf("  %s %+v %s\n", ts, path, suffix)
		}
	}
}

func tokenLitString(tok token.Token, lit string) string {
	if lit != "" {
		return lit
	}

	return tok.String()
}

// processEntryTokLits treats the 0'th entry in the tokLits as the
// latest tokLit.
func (p *fileProcessor) processEntryTokLits(ts string,
	path []string, tokLits []tokLit) string {
	x := &tokLits[0]
	if x.tok == token.INT || x.tok == token.STRING {
		name := nameFromTokLits(tokLits, 1)
		if name != "" {
			if len(path) <= 0 {
				path = strings.Split(name, " ")
				name = path[len(path)-1]
				path = path[0:len(path)-1]
			}

			fmt.Printf("  %s %s %+v %s = %s %s\n", ts, p.fname, path, name, x.tok, x.lit)

			return x.lit
		}
	}

	return ""
}

// nameFromTokLits returns the last IDENT or STRING from the tokLits,
// which the caller can use as a name.
func nameFromTokLits(tokLits []tokLit, startAt int) string {
	for i := startAt; i < len(tokLits); i++ {
		tok := tokLits[i].tok
		if tok == token.IDENT || tok == token.STRING {
			return tokLits[i].lit
		}
	}

	return ""
}
