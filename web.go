package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/gorilla/mux"
)

func (run *Run) web() {
	run.processTmp()
}

func (run *Run) webServer() {
	err := http.ListenAndServe(run.WebBind, run.webRouter())
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
		})

	r.PathPrefix("/emit/").
		Handler(http.StripPrefix("/emit/",
			http.FileServer(http.Dir(run.Tmp))))

	r.PathPrefix("/").
		Handler(http.StripPrefix("/",
			http.FileServer(http.Dir(run.WebStatic))))

	return r
}

// ------------------------------------------------------

var spaces_re = regexp.MustCompile(`\s+`)

func (run *Run) webGraph(r io.Reader) {
	fmt.Printf("webGraph... %v\n", r)

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

		ts, level, dirBaseFName, offsetByteLine, module :=
			lineParts[0], lineParts[1], lineParts[2], lineParts[3], lineParts[4]

		path := strings.Join(lineParts[5:len(lineParts)-4], " ")
		path = path[1 : len(path)-1]
		name := lineParts[len(lineParts)-4]
		val := lineParts[len(lineParts)-1]

		fmt.Printf("%s, %s, %s, %s, %s, %s\n", ts, level, dirBaseFName, offsetByteLine, module, path)
		fmt.Printf("  %s, %s\n", name, val)
	}
}
