package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

type Emitter struct {
	emitParts map[string]bool // True when that part should be emitted.
	emitTypes map[string]bool // True when that value type should be emitted.

	w io.Writer
}

func (run *Run) addEmitterFile(outDir, outName, parts, types string) (
	string, io.Closer) {
	outPath := outDir + string(os.PathSeparator) + outName
	outFile, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatal(err)
	}

	run.addEmitter(parts, types, outFile)

	return outPath, outFile
}

func (run *Run) addEmitter(parts, types string, w io.Writer) {
	run.emitters = append(run.emitters, &Emitter{
		emitParts: csvToMap(parts, map[string]bool{}),
		emitTypes: csvToMap(types, map[string]bool{}),
		w:         w,
	})
}

func (e *Emitter) emitEntryFull(ts, module, level, fnameOut, ol, linesJoined string) {
	partKind := ""
	if len(e.emitParts) > 1 {
		partKind = "FULL "
	}

	fmt.Fprintf(e.w, "  %s %s %s %s %s%s ",
		ts, level, fnameOut, ol, partKind, module)
	fmt.Fprintln(e.w, linesJoined)
}

func (e *Emitter) emitEntryPart(ts, module, level, fnameOut, ol, partKind string,
	namePath []string, name, valType, val string, valQuoted bool) {
	if e.emitParts[partKind] && e.emitTypes[valType] {
		if len(e.emitParts) <= 1 {
			partKind = ""
		} else if partKind != "" {
			partKind = partKind + " "
		}

		if name != "" {
			name = name + " "
		}

		if valQuoted {
			fmt.Fprintf(e.w, "  %s %s %s %s %s%s %+v %s= %s %q\n",
				ts, level, fnameOut, ol, partKind, module,
				namePath, name, valType, val)
		} else {
			fmt.Fprintf(e.w, "  %s %s %s %s %s%s %+v %s= %s %s\n",
				ts, level, fnameOut, ol, partKind, module,
				namePath, name, valType, val)
		}
	}
}

func csvToMap(csv string, m map[string]bool) map[string]bool {
	for _, k := range strings.Split(csv, ",") {
		m[k] = true
	}
	return m
}
