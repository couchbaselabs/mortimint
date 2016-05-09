package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
)

func (run *Run) webServer() {
	err := http.ListenAndServe(run.WebAddr, run.webRouter())
	if err != nil {
		log.Fatalf("error: http listen/serve err: %v", err)
	}
}

func (run *Run) webRouter() *mux.Router {
	fi, err := os.Stat(run.WebStatic)
	if err != nil || !fi.Mode().IsDir() {
		log.Fatal(err)
		return nil
	}

	graphData := GraphData{Data: map[string]GraphEntries{}}

	r := mux.NewRouter()

	r.HandleFunc("/progress",
		func(w http.ResponseWriter, r *http.Request) {
			run.m.Lock()
			json.NewEncoder(w).Encode(struct {
				MinTS, MaxTS string
				EmitDone     bool
				EmitProgress int64
				FileSizes    map[string]map[string]int64
				FileProgress map[string]map[string]int64
			}{
				run.minTS,
				run.maxTS,
				run.emitDone,
				run.emitProgress,
				run.fileSizes,
				run.fileProgress,
			})
			run.m.Unlock()
		}).Methods("GET")

	r.HandleFunc("/graphData",
		func(w http.ResponseWriter, r *http.Request) {
			run.m.Lock()
			json.NewEncoder(w).Encode(graphData)
			run.m.Unlock()
		}).Methods("GET")

	r.HandleFunc("/graphData",
		func(w http.ResponseWriter, r *http.Request) {
			body, err := ioutil.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}

			var graphDataIn GraphData
			err = json.Unmarshal(body, &graphDataIn)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}

			run.m.Lock()
			graphData.Add(&graphDataIn)
			if graphData.Rev < graphDataIn.Rev {
				graphData.Rev = graphDataIn.Rev
			}
			graphData.Rev++
			run.m.Unlock()
		}).Methods("POST")

	r.PathPrefix("/outDir/").
		Handler(http.StripPrefix("/outDir/",
			http.FileServer(http.Dir(run.OutDir)))).Methods("GET")

	r.PathPrefix("/").
		Handler(http.StripPrefix("/",
			http.FileServer(http.Dir(run.WebStatic)))).Methods("GET")

	return r
}

// ------------------------------------------------------

var spaces_re = regexp.MustCompile(`\s+`)

func (run *Run) webGraph(r io.Reader) {
	if run.WebAddr == "" {
		log.Fatal("error: need webAddr parameter")
		return
	}

	fmt.Printf("webGraph...\n")

	graphData := GraphData{Data: map[string]GraphEntries{}}

	lines := 0

	scanner := bufio.NewScanner(r)
	scanner.Buffer(nil, ScannerBufferCapacity)

	for scanner.Scan() {
		// Example lineStr...
		//
		//   2016-05-05T22:59:03.076 INFO \
		//   cbcollect_info_ns_1@172.23.105.190_20160506-062639/ns_server.fts.log \
		//   15122577:295 fts [managerStats "manager"] TotJanitorKickErr = INT 1
		//
		lineStr := scanner.Text()
		if !strings.HasPrefix(lineStr, "  ") {
			continue
		}

		lineParts := strings.Split(spaces_re.ReplaceAllString(lineStr[2:], " "), " ")
		if len(lineParts) < 8 ||
			lineParts[len(lineParts)-2] != "INT" ||
			lineParts[len(lineParts)-3] != "=" {
			continue
		}

		ts, level, dirFName, offsetByteLine, module :=
			lineParts[0], lineParts[1], lineParts[2], lineParts[3], lineParts[4]

		var offsetByte, offsetLine int64

		offsetByteLineParts := strings.Split(offsetByteLine, ":")
		if len(offsetByteLineParts) >= 2 {
			offsetByte, _ = strconv.ParseInt(offsetByteLineParts[0], 10, 64)
			offsetLine, _ = strconv.ParseInt(offsetByteLineParts[1], 10, 64)
		}

		path := strings.Join(lineParts[5:len(lineParts)-4], " ")
		path = path[1 : len(path)-1]
		name := lineParts[len(lineParts)-4]
		val := lineParts[len(lineParts)-1]

		graphData.Data[name] = append(graphData.Data[name], &GraphEntry{
			Ts:         ts,
			Level:      level,
			DirFName:   dirFName,
			OffSetByte: offsetByte,
			OffsetLine: offsetLine,
			Module:     module,
			Path:       path,
			Val:        val,
		})

		lines++
	}

	fmt.Printf("webGraph... lines: %d\n", lines)

	buf, err := json.Marshal(graphData)
	if err != nil {
		log.Fatal(err)
		return
	}

	fmt.Printf("webGraph... posting...\n")

	resp, err := http.Post("http://"+run.WebAddr+"/graphData",
		"application/json", bytes.NewReader(buf))
	if err != nil {
		log.Fatal(err)
		return
	}

	fmt.Println(resp.Status)
}
