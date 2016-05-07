package main

type GraphData struct {
	Rev  int64
	Data map[string][]*GraphEntry // Key'ed by name
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

func (g *GraphData) MergeData(incoming *GraphData) {
	for name, data := range incoming.Data {
		g.Data[name] = append(g.Data[name], data...)
	}
}
