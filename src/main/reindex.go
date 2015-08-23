package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"../handler"

	"github.com/bclement/boltq"
	"github.com/boltdb/bolt"
)

var dbfile = flag.String("dbfile", "", "database file, example data.db")

/*
openDatabase opens the bolt embedded database file in the provided directory
*/
func openDatabase(filename string) *bolt.DB {
	if _, err := os.Stat(filename); err != nil {
		log.Fatal(err)
	}
	db, err := bolt.Open(filename, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	return db
}

func main() {

	flag.Parse()
	if *dbfile == "" {
		fmt.Printf("missing dbfile argument\n")
		return
	}

	db := openDatabase(*dbfile)
	defer db.Close()

	ds := boltq.DataStore{db}
	comics, err := getAllComics(ds)
	if err == nil {
		err = deleteIndexes(ds)
	}
	if err == nil {
		err = index(ds, comics)
	}

	if err != nil {
		fmt.Printf("err: %v\n", err)
	}

}

func index(ds boltq.DataStore, comics []*handler.Comic) (err error) {
	err = ds.Update(func(tx *bolt.Tx) (e error) {
		for i := 0; e == nil && i < len(comics); i += 1 {
			comic := comics[i]
			/* TODO it would be better to do all this in one tx */
			fmt.Printf("Updating missing for %v, %v, %v\n", comic.SeriesId, comic.Issue, comic.CoverId)
			e = handler.TxUpdateMissingIndex(tx, *comic)
			if e == nil {
				fmt.Printf("Updating totals for %v, %v, %v\n", comic.SeriesId, comic.Issue, comic.CoverId)
				e = handler.TxUpdateComicTotals(tx, comic.SeriesId)
			}
			if e == nil {
				fmt.Printf("Updating index for %v, %v, %v\n", comic.SeriesId, comic.Issue, comic.CoverId)
				key := comic.CreateKey()
				e = handler.TxIndexComic(tx, key, comic)
			}
		}
		return
	})
	return
}

func deleteIndexes(ds boltq.DataStore) error {
	err := ds.Update(func(tx *bolt.Tx) error {
		col := []byte(handler.COMIC_COL)
		idx := []byte(handler.COMIC_INDEX)
		missing := []byte(handler.MISSING_COL)
		totals := []byte(handler.TOTALS_COL)
		e := boltq.TxDeleteIndex(tx, col, idx)
		if e == nil {
			tx.DeleteBucket(missing)
			tx.DeleteBucket(totals)
		}
		return e
	})
	return err
}

func getAllComics(ds boltq.DataStore) (comics []*handler.Comic, err error) {
	err = ds.View(func(tx *bolt.Tx) error {
		q := boltq.NewQuery([]byte("comics"), boltq.Any())
		results, e := boltq.TxQuery(tx, q)
		for i := 0; e == nil && i < len(results); i += 1 {
			var comic handler.Comic
			e = json.Unmarshal(results[i], &comic)
			if e == nil {
				comics = append(comics, &comic)
			}
		}
		return e
	})
	return
}
