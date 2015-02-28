package handler

import (
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"html/template"
	"net/http"
)

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

type PageData struct {
    Login *LoginInfo
    Quote *Quote
}

func (h HomeHandler) Handle(w http.ResponseWriter, r *http.Request) *AppError {
    login := getLoginInfo(r)
	var q Quote
	/* TODO rotate quotes */
	err := h.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("quotes"))
		if b != nil {
			encoded := b.Get([]byte("000"))
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
        return &AppError{err, "Internal Server Error", http.StatusInternalServerError}
	}

	headers := w.Header()
	headers.Add("Content-Type", "text/html")

	h.template.Execute(w, PageData{login, &q})
    return nil
}

