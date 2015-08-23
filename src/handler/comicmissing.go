package handler

import (
	"encoding/json"
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
	missingTemplate *template.Template
	ds              boltq.DataStore
	webroot         string
}

/*
ComicsMissing creates a new ComicMissingHandler
*/
func ComicsMissing(db *bolt.DB, webroot string) *Wrapper {
	missing := CreateTemplate(webroot, "base.html", "comicmissing.template")
	ds := boltq.DataStore{db}
	return &Wrapper{ComicMissingHandler{missing, ds, webroot}}
}

/*
see AppHandler interface
*/
func (h ComicMissingHandler) Handle(w http.ResponseWriter, r *http.Request,
	data PageData) *AppError {

	var err *AppError

	titles, queryErr := h.findMissing()
	if queryErr != nil {
		/* TODO update status? */
		log.Printf("Problem finding missing comics: %v", queryErr)
	}
	data["Titles"] = titles
	templateErr := h.missingTemplate.Execute(w, data)

	if templateErr != nil {
		log.Printf("Problem rendering %v\n", templateErr)
	}

	return err
}

/*
updateMissingIndex updates the missing book index for the collection
*/
func UpdateMissingIndex(ds boltq.DataStore, comic Comic) (err error) {
	err = ds.Update(func(tx *bolt.Tx) error {
		return TxUpdateMissingIndex(tx, comic)
	})
	return
}

func TxUpdateMissingIndex(tx *bolt.Tx, comic Comic) (err error) {
	compositeKey := comic.CreateKey()
	serializedKey := boltq.SerializeComposite(compositeKey)
	b, err := tx.CreateBucketIfNotExists([]byte(MISSING_COL))
	if len(comic.Books) > 0 {
		b.Delete(serializedKey)
	} else {
		/* TODO something smaller for value to save space? */
		b.Put(serializedKey, serializedKey)
	}

	return err
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
queryForMissingComics executes the provided queries on the data store
TODO this could be a generic query method
*/
func queryForMissingComics(ds boltq.DataStore, queries []*boltq.Query) (SeriesList, error) {
	rval := NewSeriesList()
	err := ds.View(func(tx *bolt.Tx) (err error) {
		for i := 0; err == nil && i < len(queries); i += 1 {
			var comic Comic
			qwrapper := QueryWrapper{queries[i]}
			results, err := qwrapper.run(tx)
			for j := 0; err == nil && j < len(results); j += 1 {
				err = json.Unmarshal(results[j], &comic)
				if err == nil {
					rval.Add(&comic)
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
