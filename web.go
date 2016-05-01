package main

import (
	"fmt"
	"log"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/gorilla/mux"
)

func (run *Run) web() {
	if run.Tmp == "" {
		tmp, err := ioutil.TempDir("", "mortimint.tmp.")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmp)

		run.Tmp = tmp
	}

	if run.EmitDict == "" {
		run.EmitDict = run.Tmp + string(os.PathSeparator) + "emit.dict"
	}

	emitLogPath := run.Tmp + string(os.PathSeparator) + "emit.log"
	emitLogFile, err := os.OpenFile(emitLogPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Fprintf(os.Stderr, "emitting: emit.dict: %s\n", run.EmitDict)
	fmt.Fprintf(os.Stderr, "emitting: emit.log: %s\n", emitLogPath)

	if run.ProgressEvery == 0 {
		run.ProgressEvery = 10000
	}

	go func() {
		run.process(emitLogFile)
		emitLogFile.Close()

		fmt.Fprintf(os.Stderr, "done: emit.dict: %s\n", run.EmitDict)
		fmt.Fprintf(os.Stderr, "done: emit.log: %s\n", emitLogPath)
		fmt.Fprintf(os.Stderr, "mortimint web (ctrl-d to exit) >> ")
	}()

	go func() {
		err := http.ListenAndServe(run.WebBind, router(run.WebStatic))
		if err != nil {
			log.Fatalf("error: http listen/serve err: %v", err)
		}
	}()

	ioutil.ReadAll(os.Stdin)
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
