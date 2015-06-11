package handler

import (
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/bclement/boltq"
	"github.com/boltdb/bolt"
)

const (
	COMIC_COL   = "comics"
	COMIC_KEY   = "comic"
	COMIC_INDEX = "_word_idx"
)

var datePattern = regexp.MustCompile("^\\s*([0-9]{4})-([0-9]{2})\\s*$")
var moneyPattern = regexp.MustCompile("^\\s*([0-9]+)(.([0-9]{2}))?\\s*$")

const (
	POOR      = "PR"
	FAIR      = "FR"
	GOOD      = "GD"
	VERY_GOOD = "VG"
	FINE      = "FN"
	VERY_FINE = "VF"
	NEAR_MINT = "NM"
)

type Book struct {
	Grade  string
	Value  int
	Signed bool
}

type Comic struct {
	CoverPath   string
	Year        int
	Month       int
	Publisher   string
	Title       string
	Subtitle    string
	Vol         int
	Issue       int
	CoverId     string
	CoverPrice  int
	Author      string
	CoverArtist string
	Pencils     string
	Inks        string
	Colors      string
	Letters     string
	Books       []Book
}

func (comic *Comic) createKey() (key [][]byte) {
	key = append(key, []byte(comic.Publisher))
	key = append(key, []byte(comic.Title))
	volKey := fmt.Sprintf("%03d", comic.Vol)
	key = append(key, []byte(volKey))
	issueKey := fmt.Sprintf("%03d", comic.Issue)
	key = append(key, []byte(issueKey))
	key = append(key, []byte(comic.CoverId))
	return
}

/*
ComicUploadHandler handles uploads to the comic page
*/
type ComicUploadHandler struct {
	loginTemplate   *template.Template
	blockedTemplate *template.Template
	uploadTemplate  *template.Template
	ds              boltq.DataStore
	webroot         string
	stopWords       map[string]bool
}

/*
ComicUpload creates a new ComicUploadHandler
*/
func ComicUpload(db *bolt.DB, webroot string) *Wrapper {

	block := CreateTemplate(webroot, "base.html", "block.template")
	login := CreateTemplate(webroot, "base.html", "login.template")
	upload := CreateTemplate(webroot, "base.html", "comicupload.template")
	ds := boltq.DataStore{db}
	stopWords := loadStopWords(ds)
	return &Wrapper{ComicUploadHandler{login, block, upload, ds, webroot, stopWords}}
}

/*
loadStopWords gets a list of words that should be ignored by the search index
*/
func loadStopWords(ds boltq.DataStore) map[string]bool {
	rval := make(map[string]bool)
	var words []string

	err := ds.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte("text-index"))
		if b != nil {
			encoded := b.Get([]byte("stop-words"))
			if encoded != nil {
				err = json.Unmarshal(encoded, &words)
			}
		}
		return err
	})
	if err != nil {
		log.Printf("Problem reading stop words from db: %v", err)
	} else {
		for _, word := range words {
			rval[word] = true
		}
	}
	return rval
}

/*
see AppHandler interface
*/
func (h ComicUploadHandler) Handle(w http.ResponseWriter, r *http.Request,
	data PageData) *AppError {

	var err *AppError

	authorized, templateErr := handleAuth(w, r, h.loginTemplate, h.blockedTemplate,
		h.ds.DB, data, "ComicUploader", "")
	if authorized && templateErr == nil {
		if r.Method == "POST" {
			status := h.process(r, data)
			data["Status"] = status
		}
		templateErr = h.uploadTemplate.Execute(w, data)
	}

	if templateErr != nil {
		log.Printf("Problem rendering %v\n", templateErr)
	}

	return err
}

func (h ComicUploadHandler) process(r *http.Request, data PageData) string {
	var status string
	var comic Comic
	var err error

	comic.Publisher, status = processString(r, "publisher", status, data)
	comic.Title, status = processString(r, "title", status, data)
	comic.Vol, status = processInt(r, "volume", status, data)
	comic.Issue, status = processInt(r, "issue", status, data)
	comic.CoverId, status = processString(r, "coverId", status, data)

	key := comic.createKey()
	comic, _, err = h.getComic(key)
	if err != nil {
		status = fmt.Sprintf("Can't lookup comic: %v", err.Error())
		/* TODO keep going? */
	}

	comic.Year, comic.Month, status = processDate(r, "date", status, data)
	comic.Subtitle, status = processString(r, "subtitle", status, data)
	comic.CoverPrice, status = processMoney(r, "coverPrice", status, data)
	comic.Author, status = processString(r, "author", status, data)
	comic.CoverArtist, status = processString(r, "coverArtist", status, data)
	comic.Pencils, status = processString(r, "pencils", status, data)
	comic.Inks, status = processString(r, "inks", status, data)
	comic.Colors, status = processString(r, "colors", status, data)
	comic.Letters, status = processString(r, "letters", status, data)
	comic.CoverPath, status = h.processCover(r, &comic)
	if status == "" {
		var book Book
		/* TODO validate grade value? */
		book.Grade = r.FormValue("grade")
		if book.Grade != "" {
			book.Value, status = processMoney(r, "value", status, data)
			signedStr := r.FormValue("signed")
			book.Signed = signedStr == "true"
			comic.Books = append(comic.Books, book)
		}
		if status == "" {
			err = h.storeComic(key, &comic)
			if err != nil {
				status = fmt.Sprintf("Unable to save comic: %v", err.Error())
			}
		}
	}

	if status == "" {
		status = "comic uploaded successfully"
	}

	return status
}

func (h ComicUploadHandler) getComic(key [][]byte) (comic Comic, found bool, err error) {
	found = false
	terms := boltq.EqAll(key)
	query := boltq.NewQuery([]byte(COMIC_COL), terms...)
	err = h.ds.View(func(tx *bolt.Tx) error {
		encoded, e := boltq.TxQuery(tx, query)
		if encoded != nil && e == nil {
			/* TODO report if dups found */
			found = true
			e = json.Unmarshal(encoded[0], comic)
		}
		return e
	})
	return
}

func (h ComicUploadHandler) storeComic(key [][]byte, comic *Comic) error {
	encoded, err := json.Marshal(comic)
	if err == nil {
		err = h.ds.Store([]byte(COMIC_COL), key, encoded)
	}

	if err == nil {
		err = h.indexString(comic.Title, key, err)
		err = h.indexString(comic.Subtitle, key, err)
		err = h.indexString(comic.Author, key, err)
		err = h.indexString(comic.CoverArtist, key, err)
		err = h.indexString(comic.Pencils, key, err)
		err = h.indexString(comic.Inks, key, err)
		err = h.indexString(comic.Colors, key, err)
		err = h.indexString(comic.Letters, key, err)
	}

	return err
}

func (h ComicUploadHandler) indexString(str string, key [][]byte, err error) error {
	parts := strings.Fields(str)
	col := []byte(COMIC_COL)
	idx := []byte(COMIC_INDEX)
	for i := 0; err == nil && i < len(parts); i += 1 {
		lower := strings.ToLower(parts[i])
		_, isStopWord := h.stopWords[lower]
		if !isStopWord {
			err = h.ds.Index(col, idx, []byte(lower), key)
		}
	}
	return err
}

func (h ComicUploadHandler) processCover(r *http.Request, comic *Comic) (coverPath, status string) {
	formFile, headers, err := r.FormFile("cover")
	if err != nil {
		if err == http.ErrMissingFile {
			status = "Missing cover file"
		} else {
			status = fmt.Sprintf("Unable to save cover: %v", err.Error())
		}
		return
	}
	dotIndex := strings.LastIndex(headers.Filename, ".")
	ext := headers.Filename[dotIndex:]
	/* TODO this assumes forward slash for file system */
	dirName := fmt.Sprintf("%v_%v_%v", comic.Publisher, comic.Title, comic.Vol)
	fileName := fmt.Sprintf("%v_%v%v", comic.Issue, comic.CoverId, ext)
	dirPath := filepath.Join("comics", "static", dirName)
	absPath := filepath.Join(h.webroot, dirPath)
	err = os.MkdirAll(absPath, 0700)
	if err == nil {
		cfilePath := filepath.Join(absPath, fileName)
		err = writeFile(cfilePath, formFile)
		coverPath = filepath.Join(dirPath, fileName)
	}
	if err != nil {
		status = err.Error()
	}
	return
}

func processField(r *http.Request, field, currStatus string, f func(string) string) (status string) {
	text := r.FormValue(field)
	if text == "" {
		status = fmt.Sprintf("Missing required field %s", field)
	} else {
		status = f(text)
	}
	/* previous messages get passed back */
	if currStatus != "" {
		status = currStatus
	}
	return
}

func processString(r *http.Request, field, currStatus string, data PageData) (value, status string) {
	status = processField(r, field, currStatus, func(text string) (status string) {
		value = text
		data[field] = text
		return
	})
	return
}

func processDate(r *http.Request, field, currStatus string, data PageData) (year, month int, status string) {
	status = processField(r, field, currStatus, func(text string) (status string) {
		groupSets := datePattern.FindAllStringSubmatch(text, -1)
		if groupSets == nil {
			status = fmt.Sprintf("Invalid date %v, expected YYYY-MM", text)
		} else {
			data[field] = text
			groups := groupSets[0]
			yearStr := groups[1]
			monthStr := groups[2]
			/* regex ensures that conversion will work */
			year, _ = strconv.Atoi(yearStr)
			month, _ = strconv.Atoi(monthStr)
		}
		return
	})
	return
}

func processInt(r *http.Request, field, currStatus string, data PageData) (value int, status string) {
	status = processField(r, field, currStatus, func(text string) (status string) {
		var err error
		value, err = strconv.Atoi(text)
		if err != nil {
			status = fmt.Sprintf("Field %s must be an integer", field)
		} else {
			data[field] = text
		}
		return
	})
	return
}

func processMoney(r *http.Request, field, currStatus string, data PageData) (totalCents int, status string) {
	status = processField(r, field, currStatus, func(text string) (status string) {
		groupSets := moneyPattern.FindAllStringSubmatch(text, -1)
		if groupSets == nil {
			status = fmt.Sprintf("Invalid value %v for field %s, expected dollars and cents", text, field)
		} else {
			data[field] = text
			groups := groupSets[0]
			dollarStr := groups[1]
			centsStr := groups[3]
			dollars, _ := strconv.Atoi(dollarStr)
			totalCents = dollars * 100
			if centsStr != "" {
				cents, _ := strconv.Atoi(centsStr)
				totalCents += cents
			}
		}
		return
	})
	return
}
