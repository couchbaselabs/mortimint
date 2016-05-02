package main

import (
	"fmt"
	"io/ioutil"
	"log"
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
	emitLogFile, err := os.OpenFile(emitLogPath,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)
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
		err := http.ListenAndServe(run.WebBind, run.webRouter())
		if err != nil {
			log.Fatalf("error: http listen/serve err: %v", err)
		}
	}()

	ioutil.ReadAll(os.Stdin)
}

func (run *Run) webRouter() *mux.Router {
	fi, err := os.Stat(run.WebStatic)
	if err != nil || !fi.Mode().IsDir() {
		log.Fatal(err)
		return nil
	}

	r := mux.NewRouter()

	r.PathPrefix("/emit/").
		Handler(http.StripPrefix("/emit/",
			http.FileServer(http.Dir(run.Tmp))))

	r.PathPrefix("/").
		Handler(http.StripPrefix("/",
			http.FileServer(http.Dir(run.WebStatic))))

	return r
}
