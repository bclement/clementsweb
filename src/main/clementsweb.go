package main

import (
	"flag"
	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
    "handler"
	"html/template"
	"log"
	"net/http"
	"net/http/fcgi"
	"os"
	"runtime"
	"strings"
	"time"
)

var standalone = flag.String("standalone", "", "binding for standalone app, example 0.0.0.0:8080")
var webroot = flag.String("webroot", "./", "root of web resource directory")
var auth = flag.Bool("auth", true, "use OAuth for login")

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}


func handleStatic(w http.ResponseWriter, r *http.Request) *handler.AppError {
	vars := mux.Vars(r)
	prepath := vars["prepath"]
	postpath := vars["postpath"]
	resourcePath := *webroot + prepath + "/static/" + postpath
	return handler.ServeFile(w, resourcePath)
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

	flag.Parse()
	var err error
	if !strings.HasSuffix(*webroot, "/") {
		*webroot += "/"
	}

    db := openDatabase(*webroot)
	defer db.Close()

    homeTemplate := createTemplate(*webroot, "home.html")
    resumeTemplate := createTemplate(*webroot, "resume.html")

    homeHandler := handler.Home(db, homeTemplate)
    staticHandler := handler.Wrapper{handler.HandlerFunc(handleStatic)}
    resumeHandler := handler.Wrapper{handler.GenericHandler{resumeTemplate}}

	r := mux.NewRouter()
	r.Handle("/", homeHandler)
    r.Handle("/resume", resumeHandler)
	r.Handle("/{prepath:.*}/static/{postpath:.*}", staticHandler)
    handler.RegisterAuth(*auth, db, r, "http://clementscode.com")

	if *standalone != "" { // run as standalone webapp
		err = http.ListenAndServe(*standalone, r)
	} else { // run in webserver via fcgi
		// nil to signify standard io
		err = fcgi.Serve(nil, r)
		logfile := setupLog(*webroot)
		defer logfile.Close()
	}
	if err != nil {
		log.Fatal(err)
	}
}
