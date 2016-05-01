package main

import (
	"log"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

func (run *Run) web() {
	go run.process(os.Stdout)

	err := http.ListenAndServe(run.WebBind, router(run.WebStatic))
	if err != nil {
		log.Fatalf("error: http listen/serve err: %v", err)
	}
}

func router(staticPath string) *mux.Router {
	fi, err := os.Stat(staticPath)
	if err != nil || !fi.Mode().IsDir() {
		log.Fatalf("error: staticPath is not a dir: %s, err: %v", staticPath, err)
	}

	r := mux.NewRouter()
	r.PathPrefix("/").Handler(http.FileServer(http.Dir(staticPath)))
	return r
}
