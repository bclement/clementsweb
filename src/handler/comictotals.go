package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/bclement/boltq"
	"github.com/boltdb/bolt"
)

const (
	TOTALS_COL = "comics_totals"
)

/*
SeriesTotal holds total count and value for a series
*/
type SeriesTotal struct {
	SeriesId string
	Count    int
	Value    int
}

/*
FormatValue formats the series total value as a currency string
*/
func (st SeriesTotal) FormatValue() string {
	return FormatCurrency(st.Value)
}

/*
FormatCurrency creates a human readable representation of the total value
*/
func FormatCurrency(totalCents int) string {
	dollars := totalCents / 100
	cents := totalCents % 100
	return fmt.Sprintf("$%d.%02d", dollars, cents)
}

/*
ComicTotalsHandler handles requests to the comics totals page
*/
type ComicTotalsHandler struct {
	totalsTemplate *template.Template
	ds             boltq.DataStore
	webroot        string
}

/*
ComicsTotals creates a new ComicTotalsHandler
*/
func ComicsTotals(db *bolt.DB, webroot string) *Wrapper {
	totals := CreateTemplate(webroot, "base.html", "comictotals.template")
	ds := boltq.DataStore{db}
	return &Wrapper{ComicTotalsHandler{totals, ds, webroot}}
}

/*
see AppHandler interface
*/
func (h ComicTotalsHandler) Handle(w http.ResponseWriter, r *http.Request,
	data PageData) *AppError {

	var err *AppError

	totals, queryErr := getComicTotals(h.ds)
	if queryErr != nil {
		/* TODO update status? */
		log.Printf("Problem finding comic totals: %v", queryErr)
	}
	data["SeriesTotals"] = totals
	totalCount := 0
	totalValue := 0
	for i := range totals {
		totalCount += totals[i].Count
		totalValue += totals[i].Value
	}
	data["TotalCount"] = totalCount
	data["TotalValue"] = FormatCurrency(totalValue)
	templateErr := h.totalsTemplate.Execute(w, data)

	if templateErr != nil {
		log.Printf("Problem rendering %v\n", templateErr)
	}

	return err
}

/*
getComicTotals gets the total values and book counts grouped by series
*/
func getComicTotals(ds boltq.DataStore) (totals []SeriesTotal, err error) {
	err = ds.View(func(tx *bolt.Tx) (e error) {
		b := tx.Bucket([]byte(TOTALS_COL))
		if b != nil {
			var total SeriesTotal
			c := b.Cursor()
			for k, v := c.First(); e == nil && k != nil; k, v = c.Next() {
				e = json.Unmarshal(v, &total)
				if e == nil {
					totals = append(totals, total)
				}
			}
		}
		return
	})
	return
}

/*
updateComicTotals updates the running totals for value and book count for the series
*/
func updateComicTotals(ds boltq.DataStore, seriesId string, book Book) (err error) {
	err = ds.Update(func(tx *bolt.Tx) error {
		var serialized []byte
		total := SeriesTotal{seriesId, 0, 0}
		b, e := tx.CreateBucketIfNotExists([]byte(TOTALS_COL))
		if e == nil {
			serialized = b.Get([]byte(seriesId))
			if serialized != nil {
				e = json.Unmarshal(serialized, &total)
			}
		}
		if e == nil {
			total.Count += 1
			total.Value += book.Value
			serialized, e = json.Marshal(&total)
			if e == nil {
				b.Put([]byte(seriesId), serialized)
			}
		}

		return e
	})
	return
}
