package main

import (
	"strconv"

	"github.com/couchbaselabs/ghistogram"
)

func MakeHistogram() *ghistogram.Histogram {
	// The 3 params are number of bins, first-bin-width, growth-factor.
	return ghistogram.NewHistogram(20, 10, 3.0)
}

// Dict represents a mapping of names to DictEntry's.
type Dict map[string]*DictEntry

type DictEntry struct {
	Kind string // For exmaple, "INT" or "STRING".
	Seen uint64 // Count of number of times this entry was seen.

	// When the Kind is "STRING", sub-dictionary of value counts.
	Vals map[string]uint64 `json:"Vals,omitempty"`

	IntHistogram *ghistogram.Histogram `json:"IntHistogram,omitempty"`
	IntTotal     int64                 `json:"IntTotal,omitempty"`
}

func (dict Dict) AddDictEntry(kind string, name, val string) {
	de := dict[name]
	if de == nil {
		de = &DictEntry{Kind: kind}

		dict[name] = de

		if kind == "STRING" && name != "median" {
			de.Vals = map[string]uint64{}
		}

		if kind == "INT" {
			de.IntHistogram = MakeHistogram()
		}
	}

	de.Seen++

	if de.Vals != nil {
		de.Vals[val]++
	}

	if de.IntHistogram != nil && val != "" {
		v, err := strconv.ParseInt(val, 10, 64)
		if err == nil && v >= 0 {
			de.IntHistogram.Add(uint64(v), 1)
			de.IntTotal += v
		}
	}
}

// AddTo adds the entries from src to dst.
func (src Dict) AddTo(dst Dict) {
	for name, srcDE := range src {
		dstDE := dst[name]
		if dstDE == nil {
			dstDE = &DictEntry{}
			dst[name] = dstDE
		}

		dstDE.Kind = srcDE.Kind
		dstDE.Seen += srcDE.Seen

		if srcDE.Vals != nil {
			if dstDE.Vals == nil {
				dstDE.Vals = map[string]uint64{}
			}
			for v, vi := range srcDE.Vals {
				dstDE.Vals[v] += vi
			}
		}

		if srcDE.IntHistogram != nil {
			if dstDE.IntHistogram == nil {
				dstDE.IntHistogram = MakeHistogram()
			}
			dstDE.IntHistogram.AddAll(srcDE.IntHistogram)
			dstDE.IntTotal += srcDE.IntTotal
		}
	}
}
