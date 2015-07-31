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
	UpToDate bool
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
	err = ds.Update(func(tx *bolt.Tx) (e error) {
		b := tx.Bucket([]byte(TOTALS_COL))
		if b != nil {
			c := b.Cursor()
			for k, v := c.First(); e == nil && k != nil; k, v = c.Next() {
				var total SeriesTotal
				e = json.Unmarshal(v, &total)
				if e == nil {
					if total.SeriesId == "" {
						total.UpToDate = false
						total.SeriesId = string(k)
					}
					if !total.UpToDate {
						total, e = calculateComicTotals(tx, total.SeriesId)
						if e == nil {
							storeTotal(b, k, v, total)
						}
					}
					totals = append(totals, total)
				}
			}
		}
		return
	})
	return
}

func storeTotal(b *bolt.Bucket, k, v []byte, st SeriesTotal) (err error) {
	v, err = json.Marshal(&st)
	if err == nil {
		b.Put(k, v)
	}
	return
}

func calculateComicTotals(tx *bolt.Tx, seriesId string) (SeriesTotal, error) {
	term := boltq.Eq([]byte(seriesId))
	q := boltq.NewQuery([]byte("comics"), term)
	results, err := boltq.TxQuery(tx, q)
	count := 0
	totalValue := 0
	for i := 0; err == nil && i < len(results); i += 1 {
		var comic Comic
		err = json.Unmarshal(results[i], &comic)
		if err == nil {
			for i := range comic.Books {
				count += 1
				totalValue += comic.Books[i].Value
			}
		}
	}
	return SeriesTotal{seriesId, count, totalValue, true}, err
}

/*
updateComicTotals updates the dirty flag for the series totals
*/
func updateComicTotals(ds boltq.DataStore, seriesId string) (err error) {
	err = ds.Update(func(tx *bolt.Tx) error {
		var serialized []byte
		total := SeriesTotal{seriesId, 0, 0, false}
		b, e := tx.CreateBucketIfNotExists([]byte(TOTALS_COL))
		if e == nil {
			serialized = b.Get([]byte(seriesId))
			if serialized != nil {
				e = json.Unmarshal(serialized, &total)
			}
		}
		if e == nil {
			total.UpToDate = false
			serialized, e = json.Marshal(&total)
			if e == nil {
				b.Put([]byte(seriesId), serialized)
			}
		}

		return e
	})
	return
}
