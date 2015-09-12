package handler

import (
	"bytes"
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
	imgPrefix    string
	storer       FileStorer
}

/*
Comics creates a new ComicViewHandler
*/
func ComicView(db *bolt.DB, webroot string, local bool) *Wrapper {
	view := CreateTemplate(webroot, "base.html", "comicview.template")
	ds := boltq.DataStore{db}
	var imgPrefix string
	var storer FileStorer
	if local {
		imgPrefix = getLocalImgPrefix(ds)
		storer = NewLocalStore(webroot)
	} else {
		var err error
		imgPrefix, err = getS3ImgPrefix(ds)
		if err != nil {
			log.Printf("Problem getting img prefix%v\n", err)
		}
		storer, err = NewS3Store(ds)
		if err != nil {
			log.Printf("Problem creating S3 store%v\n", err)
		}
	}
	return &Wrapper{ComicViewHandler{view, ds, webroot, imgPrefix, storer}}
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
			action := r.FormValue("action")
			if action == "delete comic" {
				status = processDelete(h.ds, r)
			} else if action == "clear books" {
				status = processClear(h.ds, r)
			} else {
				status = processUpload(h.ds, h.storer, r, pagedata)
			}
		}
	}

	return h.handleView(w, r, pagedata, status)
}

func processClear(ds boltq.DataStore, r *http.Request) string {
	key, status := getComicVarKey(r)
	existing, found, lookupErr := getComic(ds, key)
	if lookupErr != nil {
		status = fmt.Sprintf("Can't lookup comic: %v", lookupErr.Error())
	} else if !found {
		keyStr := formatKeys(key)
		status = fmt.Sprintf("Unable to find comic: %v", keyStr)
	} else {
		existing.Books = nil
		err := storeComic(ds, key, &existing)
		if err != nil {
			status = fmt.Sprintf("Unable to save comic: %v", err.Error())
		} else {
			totalsErr := UpdateComicTotals(ds, existing.SeriesId)
			if totalsErr != nil {
				log.Printf("Problem updating comic totals %v", totalsErr)
			}
		}
	}
	return status
}

func processDelete(ds boltq.DataStore, r *http.Request) string {
	key, status := getComicVarKey(r)

	if status == "" {
		/* TODO update missing/totals index, delete cover file */
		err := ds.Update(func(tx *bolt.Tx) error {
			return boltq.TxDelete(tx, []byte(COMIC_COL), key...)
		})
		if err != nil {
			keyStr := formatKeys(key)
			status = fmt.Sprintf("Problem deleting comic with keys %v: %v", keyStr, err)
		}
	}

	return status
}

func getComicVarKey(r *http.Request) (key [][]byte, status string) {
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

	key = [][]byte{[]byte(seriesKey), []byte(issueKey), []byte(coverKey)}
	return
}

func formatKeys(keys [][]byte) string {
	var rval bytes.Buffer
	for i := range keys {
		rval.Write(keys[i])
		rval.WriteRune(' ')
	}
	return rval.String()
}

func (h ComicViewHandler) handleView(w http.ResponseWriter, r *http.Request,
	pagedata PageData, status string) *AppError {

	var err *AppError
	var templateErr error

	key, status := getComicVarKey(r)
	existing, found, lookupErr := getComic(h.ds, key)
	if lookupErr != nil {
		status = fmt.Sprintf("Can't lookup comic: %v", lookupErr.Error())
		/* TODO keep going? */
	} else if !found {
		keyStr := formatKeys(key)
		status = fmt.Sprintf("Unable to find comic: %v", keyStr)
	}

	pagedata["ImgPrefix"] = h.imgPrefix
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
