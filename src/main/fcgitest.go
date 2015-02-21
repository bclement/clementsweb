package main

import (
    "flag"
    "github.com/gorilla/mux"
    "io"
    "log"
    "net/http"
    "net/http/fcgi"
    "runtime"
)

var standalone = flag.String("standalone", "", "binding for standalone app, example 0.0.0.0:8080")

func init() {
    runtime.GOMAXPROCS(runtime.NumCPU())
}

func handleHome(w http.ResponseWriter, r *http.Request) {
    headers := w.Header()
    headers.Add("Content-Type", "text/html")
    io.WriteString(w, "<html><body><p>It works!</p></body></html>")
}

func main() {
    r := mux.NewRouter()

    r.HandleFunc("/", handleHome)

    flag.Parse()
    var err error

    if *standalone != "" { // run as standalone webapp
        err = http.ListenAndServe(*standalone, r)
    } else { // run in webserver via fcgi
        // nil to signify standard io
        err = fcgi.Serve(nil, r)
    }
    if err != nil {
        log.Fatal(err)
    }
}

