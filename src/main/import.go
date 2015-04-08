package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/boltdb/bolt"
	"io/ioutil"
	"log"
	"os"
	"time"
)

var dbfile = flag.String("dbfile", "data.db", "database file")
var create = flag.Bool("create", false, "create db file if it doesn't exist")
var datafile = flag.String("datafile", "", "JSON encoded data for import")
var levels = flag.Int("levels", 1, "number of levels of JSON keys that should be used as bucket keys")

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

	data, err := ioutil.ReadFile(*datafile)
	if err != nil {
		log.Fatal("unable to open data file: '"+*datafile+"'\n", err)
	}

	var updates map[string]interface{}
	err = json.Unmarshal(data, &updates)
	if err != nil {
		log.Fatal(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for rootkey, value := range updates {
			bucket, err := tx.CreateBucketIfNotExists([]byte(rootkey))
			if err != nil {
				return err
			}
			entries, ok := value.(map[string]interface{})
			if !ok {
				return fmt.Errorf("Expected value of %v to be an object\n", rootkey)
			}
			err = importRecursive(1, entries, bucket)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func importRecursive(level int, entries map[string]interface{}, parent *bolt.Bucket) error {
	for key, entry := range entries {
		if level < *levels {
			child, ok := entry.(map[string]interface{})
			if !ok {
				return fmt.Errorf("Expected value of %v to be an object\n", key)
			}
			bucket, err := parent.CreateBucketIfNotExists([]byte(key))
			if err != nil {
				return err
			}
			err = importRecursive(level+1, child, bucket)
			if err != nil {
				return err
			}
		} else {
			encoded, err := json.Marshal(entry)
			if err != nil {
				return fmt.Errorf("Unable to store kvp %v: %v\n", key, entry)
			}
			parent.Put([]byte(key), encoded)
		}
	}
	return nil
}
