package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/boltdb/bolt"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

/*
HomeHandler handles home page requests
*/
type HomeHandler struct {
	template *template.Template
	db       *bolt.DB
}

/*
Home creates a new HomeHandler
*/
func Home(db *bolt.DB, webroot string) *Wrapper {
	homeTemplate := CreateTemplate(webroot, "base.html", "home.template")
	return &Wrapper{HomeHandler{homeTemplate, db}}
}

/*
Quote is used to store programmer quote information to be applied to the page template
*/
type Quote struct {
	Quote  string
	Source string
}

/*
getRandomKey returns a random database key given the highest key.
Keys are assumed to be ASCII encoded numeric values
*/
func getRandomKey(lastKey string) (string, bool) {
	index, err := strconv.Atoi(lastKey)
	if err != nil || index < 0 {
		return "", false
	}
	/* +1 to include last index in possible results */
	randIndex := rand.Intn(index + 1)
	keylen := len(lastKey)
	formatStr := fmt.Sprintf("%%0%dd", keylen)
	return fmt.Sprintf(formatStr, randIndex), true
}

/*
getRandomQuote retreives a random programmer quote from the database
*/
func getRandomQuote(db *bolt.DB) (*Quote, *AppError) {
	var q Quote
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("quotes"))
		if b != nil {
			c := b.Cursor()
			key, encoded := c.Last()
			index, ok := getRandomKey(string(key))
			if ok {
				key, randomValue := c.Seek([]byte(index))
				if key != nil {
					encoded = randomValue
				}
			}
			if encoded != nil {
				err := json.Unmarshal(encoded, &q)
				if err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		err = fmt.Errorf("Unable to get quote from db: %v", err)
		return nil, &AppError{err, "Internal Server Error", http.StatusInternalServerError}
	}
	return &q, nil
}

/*
see AppHandler interface
*/
func (h HomeHandler) Handle(w http.ResponseWriter, r *http.Request,
	data PageData) *AppError {
	quote, appErr := getRandomQuote(h.db)
	if appErr != nil {
		return appErr
	}

	headers := w.Header()
	headers.Add("Content-Type", "text/html")

	data["Quote"] = quote

	h.template.Execute(w, data)
	return nil
}
