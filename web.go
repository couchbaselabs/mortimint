package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

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
				EmitDone     bool
				EmitProgress int64
				FileSizes    map[string]map[string]int64
				FileProgress map[string]map[string]int64
			}{
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

func (run *Run) webGraph(r io.Reader) {
}
