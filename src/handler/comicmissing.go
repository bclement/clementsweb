package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"sort"

	"github.com/bclement/boltq"
	"github.com/boltdb/bolt"
)

const (
	MISSING_COL = "comics_missing"
)

/*
ComicMissingHandler handles requests to the comics missing page
*/
type ComicMissingHandler struct {
	blockedTemplate *template.Template
	loginTemplate   *template.Template
	missingTemplate *template.Template
	ds              boltq.DataStore
	webroot         string
}

/*
ComicsMissing creates a new ComicMissingHandler
*/
func ComicsMissing(db *bolt.DB, webroot string) *Wrapper {
	block := CreateTemplate(webroot, "base.html", "block.template")
	login := CreateTemplate(webroot, "base.html", "login.template")
	missing := CreateTemplate(webroot, "base.html", "comicmissing.template")
	ds := boltq.DataStore{db}
	return &Wrapper{ComicMissingHandler{block, login, missing, ds, webroot}}
}

/*
see AppHandler interface
*/
func (h ComicMissingHandler) Handle(w http.ResponseWriter, r *http.Request,
	data PageData) *AppError {

	var err *AppError

	authorized, templateErr := handleAuth(w, r, h.loginTemplate, h.blockedTemplate,
		h.ds.DB, data, "ComicUploader", "")
	if authorized && templateErr == nil {
		if r.Method == "POST" {
			status := h.process(r, data)
			data["Status"] = status
		}
		titles, queryErr := h.findMissing()
		if queryErr != nil {
			/* TODO update status? */
			log.Printf("Problem finding missing comics: %v", queryErr)
		}
		data["Titles"] = titles
		templateErr = h.missingTemplate.Execute(w, data)
	}

	if templateErr != nil {
		log.Printf("Problem rendering %v\n", templateErr)
	}

	return err
}

/*
process handles entering a book for a comic when it is added to the collection
*/
func (h ComicMissingHandler) process(r *http.Request, data PageData) (status string) {

	var comic Comic
	var err error

	comic.SeriesId, status = processString(r, "seriesId", status, data)
	comic.Issue, status = processInt(r, "issue", status, data)
	comic.CoverId, status = processString(r, "coverId", status, data)

	key := comic.createKey()
	/* TODO this isn't safe for concurrent updates */
	existing, found, err := getComic(h.ds, key)
	if err != nil {
		status = fmt.Sprintf("Can't lookup comic: %v", err.Error())
	} else if !found {
		status = fmt.Sprintf("Unable to find comic: %v, %v, %v", comic.SeriesId, comic.Issue, comic.CoverId)
	}
	if status == "" {
		var book Book
		/* TODO validate grade value? */
		book.Grade = r.FormValue("grade")
		if book.Grade != "" {
			book.Value, status = processMoney(r, "value", status, data)
			signedStr := r.FormValue("signed")
			book.Signed = signedStr == "true"
			existing.Books = append(existing.Books, book)
		}
		if status == "" {
			encoded, err := json.Marshal(existing)
			if err == nil {
				err = h.ds.Store([]byte(COMIC_COL), key, encoded)
			}
			if err == nil {
				missingErr := updateMissingIndex(h.ds, existing)
				if missingErr != nil {
					log.Printf("Problem updating missing index %v", missingErr)
				}
				totalsErr := updateComicTotals(h.ds, comic.SeriesId, book)
				if totalsErr != nil {
					log.Printf("Problem updating comic totals %v", totalsErr)
				}
			}
			if err != nil {
				status = fmt.Sprintf("Unable to save comic: %v", err.Error())
			}
		}
	}
	return
}

/*
updateMissingIndex updates the missing book index for the collection
*/
func updateMissingIndex(ds boltq.DataStore, comic Comic) (err error) {
	compositeKey := comic.createKey()
	serializedKey := boltq.SerializeComposite(compositeKey)
	err = ds.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(MISSING_COL))
		if len(comic.Books) > 0 {
			b.Delete(serializedKey)
		} else {
			/* TODO something smaller for value to save space? */
			b.Put(serializedKey, serializedKey)
		}

		return err
	})
	return
}

/*
findMissing finds all the comics that are known to exist but aren't in the collection
*/
func (h ComicMissingHandler) findMissing() ([]ComicTitle, error) {
	queries, err := h.getQueries()
	var sl SeriesList
	var titles []ComicTitle
	if err == nil {
		sl, err = queryForMissingComics(h.ds, queries)
	}
	if err == nil {
		sort.Sort(ByRelease{sl})
		titles = packageTitles(sl)
	}

	return titles, err
}

/*
queryForMissingComics executs the provided queries on the data store
TODO this could be a generic query method
*/
func queryForMissingComics(ds boltq.DataStore, queries []*boltq.Query) (SeriesList, error) {
	rval := NewSeriesList()
	err := ds.View(func(tx *bolt.Tx) (err error) {
		var comic Comic
		for i := 0; err == nil && i < len(queries); i += 1 {
			qwrapper := QueryWrapper{queries[i]}
			results, err := qwrapper.run(tx)
			for j := 0; err == nil && j < len(results); j += 1 {
				err = json.Unmarshal(results[j], &comic)
				if err == nil {
					rval.Add(comic)
				}
			}
		}

		return err
	})
	return rval, err
}

/*
getQueries creates comic queries from the entries in the missing index
*/
func (h ComicMissingHandler) getQueries() (queries []*boltq.Query, err error) {
	err = h.ds.View(func(tx *bolt.Tx) (err error) {
		b := tx.Bucket([]byte(MISSING_COL))
		if b != nil {
			c := b.Cursor()
			for k, _ := c.First(); err == nil && k != nil; k, _ = c.Next() {
				composite, err := boltq.DeserializeComposite(k)
				if err == nil {
					terms := boltq.EqAll(composite)
					q := boltq.NewQuery([]byte(COMIC_COL), terms...)
					queries = append(queries, q)
				}
			}
		}
		return
	})
	return
}
