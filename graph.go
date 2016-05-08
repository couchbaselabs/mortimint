package main

type GraphData struct {
	Rev  int64
	Runs []*GraphRun
}

type GraphRun struct {
	Data map[string][]*GraphEntry // Key'ed by name.
}

type GraphEntry struct {
	Ts         string
	Level      string
	DirFName   string
	OffSetByte int64
	OffsetLine int64
	Module     string
	Path       string
	Val        string
}

func (g *GraphData) Add(incoming *GraphData) {
	g.Runs = append(g.Runs, incoming.Runs...)
}
