package main

import (
	"encoding/json"
	"flag"
	"github.com/boltdb/bolt"
	"io/ioutil"
	"log"
	"os"
	"time"
)

var dbfile = flag.String("dbfile", "data.db", "database file")
var create = flag.Bool("create", false, "create db file if it doesn't exist")
var overwrite = flag.Bool("overwrite", false, "replace contents of bucket instead of append")
var datafile = flag.String("datafile", "", "JSON encoded data for import")

type Batch struct {
	Bucket  string
	Entries map[string]interface{}
}

func main() {

	flag.Parse()

	if !(*create) {
		if _, err := os.Stat(*dbfile); err != nil {
			log.Fatal(err)
		}
	}

	db, err := bolt.Open(*dbfile, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var updates Batch
	data, err := ioutil.ReadFile(*datafile)
	if err != nil {
		log.Fatal("unable to open data file: '"+*datafile+"'\n", err)
	}
	err = json.Unmarshal(data, &updates)
	if err != nil {
		log.Fatal(err)
	}

	db.Update(func(tx *bolt.Tx) error {
		var err error
		bucket := tx.Bucket([]byte(updates.Bucket))
		if *overwrite && bucket != nil {
			err = tx.DeleteBucket([]byte(updates.Bucket))
			if err != nil {
				return err
			}
			bucket = nil
		}
		if bucket == nil {
			bucket, err = tx.CreateBucketIfNotExists([]byte(updates.Bucket))
			if err != nil {
				return err
			}
		}
		for key, value := range updates.Entries {
			encoded, err := json.Marshal(value)
			if err == nil {
				bucket.Put([]byte(key), encoded)
			} else {
				log.Printf("Unable to store kvp %v: %v\n", key, value)
			}
		}
		return nil
	})
}
