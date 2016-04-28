package main

import (
	"math"
	"strconv"
)

// Dict represents a mapping of names to DictEntry's.
type Dict map[string]*DictEntry

type DictEntry struct {
	Kind string // For exmaple, "INT" or "STRING".
	Seen int    // Count of number of times this entry was seen.

	// When the Kind is "STRING", sub-dictionary of value counts.
	Vals map[string]int `json:"Vals,omitempty"`

	MinInt int64 `json:"MinInt,omitempty"`
	MaxInt int64 `json:"MaxInt,omitempty"`
	TotInt int64 `json:"TotInt,omitempty"`
}

func (dict Dict) AddDictEntry(kind string, name, val string) {
	d := dict[name]
	if d == nil {
		d = &DictEntry{Kind: kind, MinInt: math.MaxInt64, MaxInt: math.MinInt64}

		dict[name] = d

		if kind == "STRING" && name != "median" {
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
}

// AddTo adds the entries from src to dst.
func (src Dict) AddTo(dst Dict) {
	for name, srcDE := range src {
		dstDE := dst[name]
		if dstDE == nil {
			dstDE = &DictEntry{MinInt: math.MaxInt64, MaxInt: math.MinInt64}
			dst[name] = dstDE
		}

		dstDE.Kind = srcDE.Kind
		dstDE.Seen += srcDE.Seen

		if srcDE.Vals != nil {
			if dstDE.Vals == nil {
				dstDE.Vals = map[string]int{}
			}
			for v, vi := range srcDE.Vals {
				dstDE.Vals[v] += vi
			}
		}

		if dstDE.MinInt > srcDE.MinInt {
			dstDE.MinInt = srcDE.MinInt
		}
		if dstDE.MaxInt < srcDE.MaxInt {
			dstDE.MaxInt = srcDE.MaxInt
		}
		dstDE.TotInt += srcDE.TotInt
	}
}
