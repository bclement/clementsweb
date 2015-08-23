package handler

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/bclement/boltq"
	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
)

/*
ComicViewHandler handles requests to view a specific comic
*/
type ComicViewHandler struct {
	viewTemplate *template.Template
	ds           boltq.DataStore
	webroot      string
}

/*
Comics creates a new ComicViewHandler
*/
func ComicView(db *bolt.DB, webroot string) *Wrapper {
	view := CreateTemplate(webroot, "base.html", "comicview.template")
	ds := boltq.DataStore{db}
	return &Wrapper{ComicViewHandler{view, ds, webroot}}
}

/*
see AppHandler interface
*/
func (h ComicViewHandler) Handle(w http.ResponseWriter, r *http.Request,
	pagedata PageData) *AppError {

	var login *LoginInfo
	obj, ok := pagedata["Login"]
	if ok {
		login = obj.(*LoginInfo)
	} else {
		login = getLoginInfo(r)
	}

	var status string
	if HasRole(h.ds.DB, login.Email, "ComicUploader") {
		pagedata["Uploader"] = true
		if r.Method == "POST" {
			status = processUpload(h.ds, h.webroot, r, pagedata)
		}
	}

	return h.handleView(w, r, pagedata, status)
}

func (h ComicViewHandler) handleView(w http.ResponseWriter, r *http.Request,
	pagedata PageData, status string) *AppError {

	var err *AppError
	var templateErr error

	vars := mux.Vars(r)
	seriesKey, found := vars["series"]
	if !found {
		status = "Missing series argument"
	}
	issueKey, found := vars["issue"]
	if !found {
		status = "Missing issue argument"
	}
	coverKey, found := vars["cover"]
	if !found {
		status = "Missing cover argument"
	}

	key := [][]byte{[]byte(seriesKey), []byte(issueKey), []byte(coverKey)}
	existing, found, lookupErr := getComic(h.ds, key)
	if lookupErr != nil {
		status = fmt.Sprintf("Can't lookup comic: %v", lookupErr.Error())
		/* TODO keep going? */
	} else if !found {
		status = fmt.Sprintf("Unable to find comic: %v %v %v",
			seriesKey, issueKey, coverKey)
	}

	pagedata["Comic"] = &existing
	pagedata["Status"] = status
	templateErr = h.viewTemplate.Execute(w, pagedata)

	if templateErr != nil {
		log.Printf("Problem rendering %v\n", templateErr)
	}

	return err
}

func decodeVar(vars map[string]string, key string) (decoded string, found bool) {
	encoded, found := vars[key]
	if found {
		decoded = UnderscoreDecode(encoded)
	}
	return
}
