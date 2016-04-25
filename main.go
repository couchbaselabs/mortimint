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
	for _, dir := range os.Args[1:] {
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

	dirBase := path.Base(dir)

	for _, fileInfo := range fileInfos {
		fname := fileInfo.Name()

		fmeta, exists := FileMetas[fname]
		if !exists || fmeta.Skip {
			fmt.Fprintf(os.Stderr, "processing %s/%s, skipped\n", dirBase, fname)
			continue
		}

		fp := &fileProcessor{dir: dir, dirBase: dirBase, fname: fname, fmeta: fmeta}

		err := fp.process()
		if err != nil {
			return err
		}
	}

	return nil
}

// ------------------------------------------------------------

type fileProcessor struct {
	dir     string
	dirBase string
	fname   string
	fmeta   FileMeta

	buf []byte // Reusable buf to reduce garbage.
}

// A tokLit associates a token and a literal string.
type tokLit struct {
	tok token.Token
	lit string

	emitted bool // Marked true when this tokLit has been emitted.
}

// ------------------------------------------------------------

func (p *fileProcessor) process() error {
	fmt.Fprintf(os.Stderr,
		"processing %s/%s\n", p.dirBase, p.fname)

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

	ts := string(p.fmeta.PrefixRE.ExpandString(nil,
		"${year}-${month}-${day}T${HH}:${MM}:${SS}-${SSSS}", firstLine,
		p.fmeta.PrefixRE.FindSubmatchIndex([]byte(firstLine))))

	lines[0] = firstLine[len(match[0]):] // Strip off PrefixRE's match.

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

	s.Init(fset.AddFile(p.dir+"/"+p.fname, fset.Base(),
		len(p.buf)), p.buf, nil /* No error handler. */, 0)

	p.processEntryScanner(ts, &s, make([]string, 0, 20))
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

func (p *fileProcessor) processEntryScanner(ts string,
	s *scanner.Scanner, path []string) {
	var tokLits []tokLit
	var emitted int

	for {
		_, tok, lit := s.Scan()
		if tok == token.EOF {
			break
		}

		if skipToken[tok] {
			continue
		}

		delta, deltaExists := levelDelta[tok]
		if delta > 0 {
			pathSub := path
			pathPart := nameFromTokLits(tokLits)
			if pathPart != "" {
				pathSub = append(pathSub, pathPart)
			}

			emitted = p.emitTokLits(ts, path, tokLits, emitted)

			p.processEntryScanner(ts, s, pathSub) // Recurse on sub-level.
		} else if delta < 0 {
			break // Return from sub-level recursion.
		} else {
			// If the token is merge'able with the previous token,
			// then merge.  For example, we can merge an IDENT that's
			// followed by an adjacent IDENT.
			if !deltaExists && len(tokLits) > 0 {
				tokLitPrev := tokLits[len(tokLits)-1]
				if !tokLitPrev.emitted {
					_, prevDeltaExists := levelDelta[tokLitPrev.tok]
					if !prevDeltaExists {
						tokLits[len(tokLits)-1].lit =
							tokenLitString(tokLitPrev.tok, tokLitPrev.lit) + " " +
								tokenLitString(tok, lit)

						continue
					}
				}
			}

			tokLits = append(tokLits, tokLit{tok, lit, false})
		}
	}

	p.emitTokLits(ts, path, tokLits, emitted)
}

func (p *fileProcessor) emitTokLits(ts string, path []string,
	tokLits []tokLit, startAt int) int {
	var s []string

	for i := startAt; i < len(tokLits); i++ {
		tokLit := tokLits[i]
		if tokLit.emitted {
			continue
		}
		tokLit.emitted = true

		if tokLit.tok == token.INT {
			strs := strings.Trim(strings.Join(s, " "), "\t\n .:,")
			if len(strs) > 0 {
				fmt.Printf("  %s %s/%s STRS %+v = STRING %q\n",
					ts, p.dirBase, p.fname, path, strs)
			}

			s = nil

			name := nameFromTokLits(tokLits[0:i])

			namePath := path
			if len(namePath) <= 0 {
				namePath = strings.Split(name, " ")
				name = namePath[len(namePath)-1]
				namePath = namePath[0 : len(namePath)-1]
			}

			if len(name) > 0 {
				fmt.Printf("  %s %s/%s NAME %+v %s = %s %s\n",
					ts, p.dirBase, p.fname, namePath, name, tokLit.tok, tokLit.lit)
			}
		} else {
			s = append(s, tokenLitString(tokLit.tok, tokLit.lit))
		}
	}

	strs := strings.Trim(strings.Join(s, " "), "\t\n .:,")
	if len(strs) > 0 {
		fmt.Printf("  %s %s/%s TAIL %+v = STRING %q\n",
			ts, p.dirBase, p.fname, path, strs)
	}

	return len(tokLits)
}

// nameFromTokLits returns the last IDENT or STRING from the tokLits,
// which the caller can use as a name.
func nameFromTokLits(tokLits []tokLit) string {
	for i := len(tokLits) - 1; i >= 0; i-- {
		tok := tokLits[i].tok
		if tok == token.IDENT || tok == token.STRING {
			return tokLits[i].lit
		}
	}
	return ""
}

func tokenLitString(tok token.Token, lit string) string {
	if lit != "" {
		return lit
	}
	return tok.String()
}
