package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
	"html/template"
	"io"
	"log"
	"mime"
	"net/http"
	"net/http/fcgi"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var standalone = flag.String("standalone", "", "binding for standalone app, example 0.0.0.0:8080")
var webroot = flag.String("webroot", "./", "root of web resource directory")

var homeTemplate *template.Template

var db *bolt.DB

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

type Quote struct {
	Quote  string
	Source string
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	var q Quote
	/* TODO rotate quotes */
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("quotes"))
		if b != nil {
			encoded := b.Get([]byte("000"))
			if encoded != nil {
				err := json.Unmarshal(encoded, &q)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		log.Print("Unable to get quote from db", err)
		sendError(w, http.StatusInternalServerError, "Internal Server Error")
	}

	headers := w.Header()
	headers.Add("Content-Type", "text/html")

	homeTemplate.Execute(w, q)
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

func setupLog(logdir string) *os.File {
	logfile := logdir + "log.txt"
	f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Unable to open log file "+logfile, err)
	}
	log.SetOutput(f)
	return f
}

func openDatabase(dbdir string) *bolt.DB {
	dbfile := dbdir + "data.db"
	if _, err := os.Stat(dbfile); err != nil {
		log.Fatal(err)
	}
	db, err := bolt.Open(dbfile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func createTemplate(webroot string, templateFile string) *template.Template {
	fname := webroot + "templates/" + templateFile
	rval, err := template.ParseFiles(fname)
	if err != nil {
		log.Fatal(err)
	}
	return rval
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

	db = openDatabase(*webroot)
	defer db.Close()

	homeTemplate = createTemplate(*webroot, "home.html")

	if *standalone != "" { // run as standalone webapp
		err = http.ListenAndServe(*standalone, r)
	} else { // run in webserver via fcgi
		logfile := setupLog(*webroot)
		defer logfile.Close()
		// nil to signify standard io
		err = fcgi.Serve(nil, r)
	}
	if err != nil {
		log.Fatal(err)
	}
}
