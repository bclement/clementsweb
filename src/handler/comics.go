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
	"github.com/gorilla/mux"
)

/*
Query is an interface to abstract out how data is requested (string search vs key lookup)
*/
type Query interface {
	/*
		run executes the query and returns the resulting data
	*/
	run(tx *bolt.Tx) ([][]byte, error)
}

/*
IndexQuery encapsulates a string search of the database
*/
type IndexQuery struct {
	collection []byte
	index      []byte
	values     [][]byte
}

/*
newIndexQuery creates a new string search from the fields of the qstring
*/
func newIndexQuery(ds boltq.DataStore, qstring string) IndexQuery {
	var tokens [][]byte
	ds.View(func(tx *bolt.Tx) error {
		tokens = normalizeIndexTokens(tx, qstring)
		return nil
	})
	col := []byte(COMIC_COL)
	idx := []byte(COMIC_INDEX)
	return IndexQuery{col, idx, tokens}
}

/*
see Query interface
*/
func (iq IndexQuery) run(tx *bolt.Tx) ([][]byte, error) {
	return boltq.TxIndexQuery(tx, iq.collection, iq.index, iq.values...)
}

/*
QueryWrapper wraps a boltq query in the Query interface
*/
type QueryWrapper struct {
	q *boltq.Query
}

/*
see Query interface
*/
func (qw QueryWrapper) run(tx *bolt.Tx) ([][]byte, error) {
	return boltq.TxQuery(tx, qw.q)
}

/*
ComicList is a sortable slice of comics
*/
type ComicList []*Comic

/*
see Sort interface
*/
func (cl ComicList) Len() int {
	return len(cl)
}

/*
see Sort interface
*/
func (cl ComicList) Less(i, j int) bool {
	if cl[i].Issue == cl[j].Issue {
		return cl[i].CoverId < cl[j].CoverId
	} else {
		return cl[i].IssueValue() < cl[j].IssueValue()
	}
}

/*
see Sort interface
*/
func (cl ComicList) Swap(i, j int) {
	cl[i], cl[j] = cl[j], cl[i]
}

/*
ComicTitle groups comics in the same series that have the same title on the cover
*/
type ComicTitle struct {
	Publisher   string
	DisplayName string
	Path        string
	Comics      ComicList
}

/*
SeriesList is a sortable list of comics grouped by common numbering continuities (SeriesId).
Most often all comics in a series have the same title... but not always.
*/
type SeriesList struct {
	Map  map[string]ComicList
	Keys []string
}

/*
NewSeriesList creates a new SeriesList
*/
func NewSeriesList() SeriesList {
	return SeriesList{make(map[string]ComicList), nil}
}

/*
Adds the comic to the appropriate series
*/
func (sl *SeriesList) Add(c *Comic) {
	sl.Map[c.SeriesId] = append(sl.Map[c.SeriesId], c)
	sl.Keys = sl.Keys[:0]
	for key, _ := range sl.Map {
		sl.Keys = append(sl.Keys, key)
	}
}

/*
see Sort interface
*/
func (sl SeriesList) Len() int {
	return len(sl.Keys)
}

/*
see Sort interface
*/
func (sl SeriesList) Swap(i, j int) {
	sl.Keys[i], sl.Keys[j] = sl.Keys[j], sl.Keys[i]
}

/*
FirstOf returns the first comic in the series with index i
*/
func (sl SeriesList) FirstOf(i int) *Comic {
	return sl.Map[sl.Keys[i]][0]
}

/*
ByRelease is a wrapper that sorts the series list by calendar release date
*/
type ByRelease struct {
	SeriesList
}

/*
see Sort interface
*/
func (b ByRelease) Less(i, j int) bool {
	one, two := b.FirstOf(i), b.FirstOf(j)
	comp := one.Year - two.Year
	if comp == 0 {
		comp = one.Month - two.Month
	}
	return comp < 0
}

/*
ByChron is a wrapper that sorts the series list by story chronology
*/
type ByChron struct {
	SeriesList
}

/*
see Sort interface
*/
func (b ByChron) Less(i, j int) bool {
	one, two := b.FirstOf(i), b.FirstOf(j)
	return one.ChronOffset < two.ChronOffset
}

/*
ComicHandler handles requests to the comics page
*/
type ComicHandler struct {
	listTemplate   *template.Template
	topTemplate    *template.Template
	seriesTemplate *template.Template
	ds             boltq.DataStore
	webroot        string
	imgPrefix      string
}

/*
Comics creates a new ComicHandler
*/
func Comics(db *bolt.DB, webroot string, local bool) *Wrapper {
	list := CreateTemplate(webroot, "base.html", "comiclist.template")
	top := CreateTemplate(webroot, "base.html", "comictop.template")
	series := CreateTemplate(webroot, "base.html", "comicseries.template")
	ds := boltq.DataStore{db}
	var imgPrefix string
	if local {
		imgPrefix = getLocalImgPrefix(ds)
	} else {
		var err error
		imgPrefix, err = getS3ImgPrefix(ds)
		if err != nil {
			log.Printf("Problem getting img prefix: %v\n", err)
		}
	}
	return &Wrapper{ComicHandler{list, top, series, ds, webroot, imgPrefix}}
}

func getLocalImgPrefix(ds boltq.DataStore) string {
	/* TODO this should be configurable */
	return "/static/comics"
}

func getS3ImgPrefix(ds boltq.DataStore) (prefix string, err error) {
	ds.View(func(tx *bolt.Tx) error {
		/* TODO this should go in a constant */
		b := tx.Bucket([]byte("aws.config"))
		if b != nil {
			bytes := b.Get([]byte("imgPrefix"))
			if bytes != nil {
				prefix = string(bytes)
			}
		}
		return nil
	})
	if prefix == "" {
		err = fmt.Errorf("Unable to find imgPrefix in aws config")
	}

	return
}

/*
see AppHandler interface
*/
func (h ComicHandler) Handle(w http.ResponseWriter, r *http.Request,
	pagedata PageData) *AppError {

	var err *AppError
	var templateErr error
	var template *template.Template
	var q Query
	vars := mux.Vars(r)
	series, seriesPresent := vars["series"]
	issueStr, issuePresent := vars["issue"]
	qstring := r.FormValue("q")
	topSeries := r.FormValue("s")
	if seriesPresent {
		template = h.seriesTemplate
		terms := []*boltq.Term{boltq.Eq([]byte(series))}
		if issuePresent {
			terms = append(terms, boltq.Eq([]byte(issueStr)))
		}
		q = QueryWrapper{boltq.NewQuery([]byte("comics"), terms...)}
	} else if qstring != "" {
		template = h.listTemplate
		q = newIndexQuery(h.ds, qstring)
		pagedata["query"] = qstring
	} else if topSeries != "" {
		template = h.listTemplate
		term := boltq.Eq([]byte(topSeries))
		q = QueryWrapper{boltq.NewQuery([]byte("comics"), term)}
	} else {
		template = h.topTemplate
		terms := []*boltq.Term{boltq.Any(), boltq.Eq([]byte("1"))}
		q = QueryWrapper{boltq.NewQuery([]byte("comics"), terms...)}
	}
	sl, e := getComics(h.ds, q)
	if e == nil {
		sort.Sort(ByRelease{sl})
		titles := packageTitles(sl)
		pagedata["Titles"] = titles
		pagedata["ImgPrefix"] = h.imgPrefix
		templateErr = template.Execute(w, pagedata)
	} else {
		e = fmt.Errorf("Unable to get comics from db: %v", e)
		err = &AppError{e, "Internal Server Error", http.StatusInternalServerError}
	}

	if templateErr != nil {
		log.Printf("Problem rendering %v\n", templateErr)
	}

	return err
}

/*
packageTitles sorts the series and packages them in bundles that share the same title
*/
func packageTitles(sl SeriesList) (titles []ComicTitle) {
	for _, seriesId := range sl.Keys {
		list := sl.Map[seriesId]
		/* ensure that issues are in order */
		sort.Sort(list)
		/* TODO this is only needed because of 1997 star wars,
		it should be optimized for common case */
		firstTitle := list[0].Title
		path := list[0].SeriesPath()
		currTitle := ComicTitle{list[0].Publisher, seriesId, path, nil}
		for i := range list {
			if firstTitle != list[i].Title {
				titles = append(titles, currTitle)
				currTitle = ComicTitle{list[i].Publisher, list[i].Title, seriesId, nil}
			}
			currTitle.Comics = append(currTitle.Comics, list[i])
		}
		titles = append(titles, currTitle)
	}
	return
}

/*
getComics executes a query for comics and populates a SeriesList with the results
*/
func getComics(ds boltq.DataStore, query Query) (SeriesList, error) {
	rval := NewSeriesList()
	err := ds.View(func(tx *bolt.Tx) error {
		results, e := query.run(tx)
		for i := 0; e == nil && i < len(results); i += 1 {
			var comic Comic
			e = json.Unmarshal(results[i], &comic)
			if e == nil {
				rval.Add(&comic)
			}
		}

		return e
	})
	return rval, err
}
