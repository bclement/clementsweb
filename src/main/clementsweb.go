package main

import (
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var standalone = flag.String("standalone", "", "binding for standalone app, example 0.0.0.0:8080")
var webroot = flag.String("webroot", "./", "root of web resource directory")

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

func handleStatic(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	prepath := vars["prepath"]
	postpath := vars["postpath"]
	resourcePath := *webroot + prepath + "/static/" + postpath
	serveFile(w, resourcePath)
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	serveFile(w, *webroot+"templates/home.html")
}

func serveFile(w http.ResponseWriter, fname string) {
	/* FIXME ensure path is sanitary */
	f, err := os.Open(fname)
	if err != nil {
		if os.IsNotExist(err) {
			sendError(w, http.StatusNotFound, "File Not Found")
		} else {
			sendError(w, http.StatusInternalServerError, "Internal Server Error")
		}
	} else {
		writeContentHeader(w, fname)
		io.Copy(w, f)
	}
}

func writeContentHeader(w http.ResponseWriter, fname string) {
	extension := filepath.Ext(fname)
	if extension != "" {
		mimetype := mime.TypeByExtension(extension)
		if mimetype != "" {
			headers := w.Header()
			headers.Add("Content-Type", mimetype)
		}
	}
}

func sendError(w http.ResponseWriter, status int, msg string) {
	w.WriteHeader(status)
	fmt.Fprint(w, msg)
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/", handleHome)
	r.HandleFunc("/{prepath:.*}/static/{postpath:.*}", handleStatic)

	flag.Parse()
	var err error
	if !strings.HasSuffix(*webroot, "/") {
		*webroot += "/"
	}

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
