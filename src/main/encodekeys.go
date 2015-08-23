package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"../handler"

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

	err := db.Update(func(tx *bolt.Tx) (e error) {
		b := tx.Bucket([]byte("comics"))
		if b != nil {
			e = b.ForEach(func(k, v []byte) (seriesErr error) {
				oldSeriesBucket := b.Bucket(k)
				seriesId := string(k)
				encodedSeries := handler.SanitizeKey(seriesId)
				var newSeriesBucket *bolt.Bucket
				if seriesId != encodedSeries {
					newSeriesBucket, seriesErr = b.CreateBucket([]byte(encodedSeries))
				} else {
					newSeriesBucket = oldSeriesBucket
				}
				if seriesErr == nil {
					seriesErr = processSeries(oldSeriesBucket, newSeriesBucket)
				}
				if seriesErr == nil && seriesId != encodedSeries {
					seriesErr = b.DeleteBucket(k)
				}
				return
			})
		}
		return
	})

	if err != nil {
		fmt.Printf("err: %v\n", err)
	}

}

func processSeries(oldSeriesBucket, newSeriesBucket *bolt.Bucket) error {
	err := oldSeriesBucket.ForEach(func(k, v []byte) error {
		newIssueKey := cleanKey(k)
		oldIssueBucket := oldSeriesBucket.Bucket(k)
		newIssueBucket, e := newSeriesBucket.CreateBucket(newIssueKey)
		if e == nil {
			e = processLevel(oldIssueBucket, newIssueBucket)
		}
		return e
	})
	return err
}

func cleanKey(key []byte) []byte {
	keyStr := string(key)
	i, _ := strconv.Atoi(keyStr)
	keyStr = fmt.Sprintf("%v", i)
	return []byte(keyStr)
}

func processLevel(oldBucket, newBucket *bolt.Bucket) (err error) {
	err = oldBucket.ForEach(func(k, v []byte) (e error) {
		if v == nil {
			next := oldBucket.Bucket(k)
			newNext, e := newBucket.CreateBucket(k)
			if next != nil && e == nil {
				e = processLevel(next, newNext)
			}
		} else {
			updated, err := updateComic(v)
			if err == nil {
				newBucket.Put(k, updated)
			}
		}
		return
	})
	return
}

func updateComic(val []byte) (rval []byte, err error) {
	comic := make(map[string]interface{})
	rval = val
	err = json.Unmarshal(val, &comic)
	if err == nil {
		issueNum := comic["Issue"].(float64)
		issueStr := fmt.Sprintf("%v", issueNum)
		comic["Issue"] = issueStr
		rval, err = json.Marshal(&comic)
	}
	return
}
