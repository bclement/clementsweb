package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"../handler"

	"github.com/boltdb/bolt"
)

var dbfile = flag.String("dbfile", "", "database file, example data.db")
var mapstr = flag.String("mapping", "GD:FN,VG:VF", "mapping string, example GD:FN,VG:VF")

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

	mapping := make(map[string]string)
	pairs := strings.Split(*mapstr, ",")
	for i := range pairs {
		kvp := strings.Split(pairs[i], ":")
		if len(kvp) != 2 {
			fmt.Printf("invalid mapping %v at %v\n", mapstr, pairs[i])
			return
		}
		mapping[kvp[0]] = kvp[1]
	}

	db := openDatabase(*dbfile)
	defer db.Close()

	err := db.Update(func(tx *bolt.Tx) (err error) {
		b := tx.Bucket([]byte("comics"))
		if b != nil {
			err = processLevel(b, mapping)
		}
		return
	})

	if err != nil {
		fmt.Printf("err: %v\n", err)
	}

}

func processLevel(b *bolt.Bucket, mapping map[string]string) (err error) {
	c := b.Cursor()
	for k, v := c.First(); err == nil && k != nil; k, v = c.Next() {
		if v == nil {
			next := b.Bucket(k)
			if next != nil {
				err = processLevel(next, mapping)
			}
		} else {
			updated, err := updateComic(v, mapping)
			if err == nil {
				b.Put(k, updated)
			}
		}
	}
	return
}

func updateComic(val []byte, mapping map[string]string) (rval []byte, err error) {
	var comic handler.Comic
	rval = val
	err = json.Unmarshal(val, &comic)
	if err == nil {
		for i := range comic.Books {
			replacement, found := mapping[comic.Books[i].Grade]
			if found {
				comic.Books[i].Grade = replacement
			}
		}
		rval, err = json.Marshal(&comic)
	}
	return
}
