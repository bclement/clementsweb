package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/fcgi"
	"os"
	"runtime"
	"strings"
	"time"

	"../handler"
	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
)

var standalone = flag.String("standalone", "", "binding for standalone app, example 0.0.0.0:8080")
var webroot = flag.String("webroot", "./", "root of web resource directory")
var auth = flag.Bool("auth", true, "use OAuth for login")

func init() {
	runtime.GOMAXPROCS(runtime.NumCPU())
}

/*
setupLog opens the log file for the webserver
*/
func setupLog(logdir string) *os.File {
	logfile := logdir + "log.txt"
	f, err := os.OpenFile(logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatal("Unable to open log file "+logfile, err)
	}
	log.SetOutput(f)
	return f
}

/*
openDatabase opens the bolt embedded database file in the provided directory
*/
func openDatabase(dbdir string) *bolt.DB {
	/* TODO database file name should be configurable */
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

func main() {

	flag.Parse()
	var err error
	if !strings.HasSuffix(*webroot, "/") {
		*webroot += "/"
	}

	db := openDatabase(*webroot)
	defer db.Close()

	resumeTemplate := handler.CreateTemplate(*webroot, "base.html", "resume.template")
	projectsTemplate := handler.CreateTemplate(*webroot, "base.html", "projects.template")

	homeHandler := handler.Home(db, *webroot)
	staticHandler := http.FileServer(http.Dir(*webroot))
	resumeHandler := handler.Wrapper{handler.GenericHandler{resumeTemplate}}
	projectsHandler := handler.Wrapper{handler.GenericHandler{projectsTemplate}}
	videosHandler := handler.Videos(db, *webroot)
	vidUploadHandler := handler.VideoUpload(db, *webroot)
	vidSubHandler := handler.VideoSub(db, *webroot)
	missingHandler := handler.Missing(*webroot)
	adminHandler := handler.Admin(db, *webroot)
	comicUploadHandler := handler.ComicUpload(db, *webroot)
	comicHandler := handler.Comics(db, *webroot)
	comicMissingHandler := handler.ComicsMissing(db, *webroot)
	comicTotalsHandler := handler.ComicsTotals(db, *webroot)

	r := mux.NewRouter()
	r.Handle("/", homeHandler)
	r.Handle("/admin", adminHandler)
	r.Handle("/resume", resumeHandler)
	r.Handle("/resume/", resumeHandler)
	r.Handle("/projects", projectsHandler)
	r.Handle("/projects/", projectsHandler)
	r.Handle("/comics", handler.Redirect("comics/"))
	r.Handle("/comics/", comicHandler)
	r.Handle("/comics/upload", comicUploadHandler)
	r.Handle("/comics/missing", comicMissingHandler)
	r.Handle("/comics/totals", comicTotalsHandler)
	r.Handle("/comics/{series:.*}", comicHandler)
	r.Handle("/videos", handler.Redirect("videos/"))
	r.Handle("/videos/", videosHandler)
	r.Handle("/videos/upload", vidUploadHandler)
	r.Handle("/videos/subscription", vidSubHandler)
	r.Handle("/videos/{path:.*}", videosHandler)
	r.Handle("/static/{path:.*}", staticHandler)
	r.NotFoundHandler = missingHandler
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
