package main

import (
	"strconv"

	"github.com/couchbaselabs/ghistogram"
)

func MakeHistogram() *ghistogram.Histogram {
	// The 3 params are number of bins, first-bin-width, growth-factor.
	return ghistogram.NewHistogram(20, 10, 3.0)
}

// ------------------------------------------------------------

// Dict represents a mapping of names to DictEntry's.
type Dict map[string]*DictEntry

type DictEntry struct {
	Kind string // For exmaple, "INT" or "STRING".
	Seen uint64 // Count of number of times this entry was seen.

	// When the Kind is "STRING", sub-dictionary of value counts.
	Vals map[string]uint64 `json:"Vals,omitempty"`

	IntHistogram *ghistogram.Histogram `json:"IntHistogram,omitempty"`
}

func MakeDictEntry(kind string) *DictEntry {
	return &DictEntry{
		Kind:         kind,
		Vals:         map[string]uint64{},
		IntHistogram: MakeHistogram(),
	}
}

func (dict Dict) AddDictEntry(kind string, name, val string) {
	de := dict[name]
	if de == nil {
		de = MakeDictEntry(kind)
		dict[name] = de
	}

	de.Seen++

	if kind == "STRING" {
		de.Vals[val]++
	}

	v, err := strconv.ParseInt(val, 10, 64)
	if err == nil && v >= 0 {
		de.IntHistogram.Add(uint64(v), 1)
	}
}

// AddTo adds the entries from src to dst.
func (src Dict) AddTo(dst Dict) {
	for name, srcDE := range src {
		dstDE := dst[name]
		if dstDE == nil {
			dstDE = MakeDictEntry(srcDE.Kind)
			dst[name] = dstDE
		}

		dstDE.Seen += srcDE.Seen
		for v, vi := range srcDE.Vals {
			dstDE.Vals[v] += vi
		}
		dstDE.IntHistogram.AddAll(srcDE.IntHistogram)
	}
}
