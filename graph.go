package main

import (
	"sort"
)

type GraphData struct {
	Rev  int64
	Data map[string]GraphEntries // Key'ed by name.
}

type GraphEntries []*GraphEntry

func (a GraphEntries) Len() int           { return len(a) }
func (a GraphEntries) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a GraphEntries) Less(i, j int) bool { return a[i].Ts < a[j].Ts }

type GraphEntry struct {
	Ts         string
	Level      string
	DirFName   string
	OffsetByte int64
	OffsetLine int64
	Module     string
	Path       string
	Val        string
}

func (g *GraphData) Add(incoming *GraphData) {
	for name, entries := range incoming.Data {
		g.Data[name] = append(g.Data[name], entries...)

		sort.Sort(g.Data[name])
	}
}
