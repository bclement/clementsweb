package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"../handler"

	"github.com/boltdb/bolt"
)

var dbfile = flag.String("dbfile", "", "database file, example data.db")
var base = flag.String("base", "", "base directory for cover files")

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
	if *base == "" {
		fmt.Printf("missing base directory argument\n")
		return
	}

	db := openDatabase(*dbfile)
	defer db.Close()

	err := db.Update(func(tx *bolt.Tx) (err error) {
		b := tx.Bucket([]byte("comics"))
		if b != nil {
			err = processLevel(b)
		}
		return
	})

	if err != nil {
		fmt.Printf("err: %v\n", err)
	}

}

func processLevel(b *bolt.Bucket) (err error) {
	c := b.Cursor()
	for k, v := c.First(); err == nil && k != nil; k, v = c.Next() {
		if v == nil {
			next := b.Bucket(k)
			if next != nil {
				err = processLevel(next)
			}
		} else {
			updated, err := updateComic(v)
			if err == nil {
				b.Put(k, updated)
			}
		}
	}
	return
}

func updateComic(val []byte) (rval []byte, err error) {
	/* TODO this should come from the file */
	ext := ".jpg"
	var comic handler.Comic
	rval = val
	err = json.Unmarshal(val, &comic)
	if err == nil {
		dirName := comic.SeriesKey()
		issuePart := comic.IssueKey()
		coverPart := comic.CoverKey()
		fileName := fmt.Sprintf("%v_%v%v", issuePart, coverPart, ext)
		coverPath := filepath.Join(dirName, fileName)
		if coverPath != comic.CoverPath {
			absDir := filepath.Join(*base, dirName)
			absFile := filepath.Join(*base, coverPath)
			err = os.MkdirAll(absDir, 0700)
			if err == nil {
				oldPath := filepath.Join(*base, comic.CoverPath)
				err = os.Rename(oldPath, absFile)
			}
			if err == nil {
				comic.CoverPath = coverPath
				rval, err = json.Marshal(&comic)
			}
		}
	}
	return
}
