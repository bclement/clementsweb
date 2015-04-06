package handler

import (
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"html/template"
    "math/rand"
	"net/http"
    "strconv"
    "time"
)

func init() {
    rand.Seed(time.Now().UTC().UnixNano())
}

type HomeHandler struct {
    template *template.Template
    db *bolt.DB
}

func Home(db *bolt.DB, t *template.Template) *Wrapper {
    return &Wrapper{HomeHandler{t, db}}
}

type Quote struct {
	Quote  string
	Source string
}

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

func (h HomeHandler) Handle(w http.ResponseWriter, r *http.Request,
        pagedata map[string]interface{}) *AppError {
    quote, appErr := getRandomQuote(h.db)
	if appErr != nil {
        return appErr
	}

	headers := w.Header()
	headers.Add("Content-Type", "text/html")

    pagedata["Quote"] = quote

	h.template.Execute(w, pagedata)
    return nil
}

