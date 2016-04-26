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
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"os"
	"path"
	"strconv"
	"strings"

	"go/scanner"
	"go/token"
)

var ScannerBufferCapacity = 20 * 1024 * 1024

func main() {
	parseArgs(os.Args).process()
}

// ------------------------------------------------------------

// Run is the main data struct that describes a processing run.
type Run struct {
	DictPath  string   // Path to optional dictionary file to output.
	EmitOrig  bool     // When true, also emit original log entries to stdout.
	EmitParts string   // Comma-separated list of parts of data to emit (NAME, STRS, TAIL).
	EmitTypes string   // Comma-separated list of value types to emit (INT, STRING),
	Verbose   int      // More verbosity when number is greater.
	Dirs      []string // Directories to process.

	emitParts map[string]bool // True when that part should be emitted.
	emitTypes map[string]bool // True when that value type should be emitted.

	dict map[string]*DictEntry
}

func parseArgs(args []string) *Run {
	run := &Run{
		emitParts: map[string]bool{},
		emitTypes: map[string]bool{},
		dict:      map[string]*DictEntry{},
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
	flagSet.IntVar(&run.Verbose, "v", 0,
		"optional, use a higher number for more verbose stderr logging")
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

		fp := &fileProcessor{run: run, dir: dir, dirBase: dirBase, fname: fname, fmeta: fmeta}

		err := fp.process()
		if err != nil {
			return err
		}
	}

	return nil
}

// ------------------------------------------------------------

type DictEntry struct {
	Kind string // For exmaple, "INT" or "STRING".
	Seen int    // Count of number of times this entry was seen.

	// When kind is "STRING", sub-dictionary of value counts.
	Vals map[string]int `json:"Vals,omitempty"`

	MinInt int64 `json:"MinInt,omitempty"`
	MaxInt int64 `json:"MaxInt,omitempty"`
	TotInt int64 `json:"TotInt,omitempty"`
}

func (run *Run) AddDictEntry(kind string, name, val string) string {
	d := run.dict[name]
	if d == nil {
		d = &DictEntry{Kind: kind, MinInt: math.MaxInt64, MaxInt: math.MinInt64}

		run.dict[name] = d

		if kind == "STRING" {
			d.Vals = map[string]int{}
		}
	}

	d.Seen++

	if d.Vals != nil {
		d.Vals[val]++
	}

	if d.Kind == "INT" && val != "" {
		v, err := strconv.ParseInt(val, 10, 64)
		if err == nil {
			if d.MinInt > v {
				d.MinInt = v
			}
			if d.MaxInt < v {
				d.MaxInt = v
			}
			d.TotInt += v
		}
	}

	return name
}

// ------------------------------------------------------------

type fileProcessor struct {
	run     *Run
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
	fmt.Fprintf(os.Stderr, "processing %s/%s\n", p.dirBase, p.fname)

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

		if p.fmeta.EntryStart == nil || p.fmeta.EntryStart(lineStr) {
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

	if p.run.EmitOrig {
		for _, line := range lines {
			fmt.Println(line)
		}
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

	p.processEntryScanner(startOffset, startLine, ts, &s, make([]string, 0, 20))
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

	token.ADD:       0, // +
	token.SUB:       0, // -
	token.MUL:       0, // *
	token.QUO:       0, // /
	token.COLON:     0,
	token.COMMA:     0,
	token.PERIOD:    0,
	token.SEMICOLON: 0,
}

var skipToken = map[token.Token]bool{
	token.SHL: true, // <<
	token.SHR: true, // >>
}

func (p *fileProcessor) processEntryScanner(startOffset, startLine int,
	ts string, s *scanner.Scanner, path []string) {
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

			emitted = p.emitTokLits(startOffset, startLine, ts, path,
				tokLits, emitted)

			// Recurse on sub-level.
			p.processEntryScanner(startOffset, startLine, ts, s, pathSub)
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

	p.emitTokLits(startOffset, startLine, ts, path, tokLits, emitted)
}

func (p *fileProcessor) emitTokLits(startOffset, startLine int, ts string,
	path []string, tokLits []tokLit, startAt int) int {
	var s []string

	for i := startAt; i < len(tokLits); i++ {
		tokLit := tokLits[i]
		if tokLit.emitted {
			continue
		}
		tokLit.emitted = true

		tokStr := tokLit.tok.String()
		if p.run.emitTypes[tokStr] {
			strs := strings.Trim(strings.Join(s, " "), "\t\n .:,")
			if p.run.emitParts["STRS"] && len(strs) > 0 {
				fmt.Printf("  %s %s/%s:%d:%d STRS %+v = STRING %q\n",
					ts, p.dirBase, p.fname, startOffset, startLine, path, strs)
			}

			s = nil

			name := validateName(nameFromTokLits(tokLits[0:i]))
			if name != "" {
				namePath := path
				if len(namePath) <= 0 {
					namePath = strings.Split(name, " ")
					name = namePath[len(namePath)-1]
					namePath = namePath[0 : len(namePath)-1]
				}

				if len(name) > 0 {
					name = p.run.AddDictEntry(tokStr, name, tokLit.lit)
					if name != "" && p.run.emitParts["NAME"] {
						fmt.Printf("  %s %s/%s:%d:%d NAME %+v %s = %s %s\n",
							ts, p.dirBase, p.fname, startOffset, startLine,
							namePath, name, tokLit.tok, tokLit.lit)
					}
				}
			}
		} else {
			s = append(s, tokenLitString(tokLit.tok, tokLit.lit))
		}
	}

	strs := strings.Trim(strings.Join(s, " "), "\t\n .:,")
	if p.run.emitParts["TAIL"] && len(strs) > 0 {
		fmt.Printf("  %s %s/%s:%d:%d TAIL %+v = STRING %q\n",
			ts, p.dirBase, p.fname, startOffset, startLine, path, strs)
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

func validateName(name string) string {
	name = strings.Trim(name, " \t\n\"")
	if strings.IndexAny(name, "<>/ ") >= 0 {
		return ""
	}
	return name
}

func tokenLitString(tok token.Token, lit string) string {
	if lit != "" {
		return lit
	}
	return tok.String()
}
