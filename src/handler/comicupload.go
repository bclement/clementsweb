package handler

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"html/template"
	"io"
	"log"
	"math/big"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bclement/boltq"
	"github.com/boltdb/bolt"
)

const (
	COMIC_COL   = "comics"
	COMIC_KEY   = "comic"
	COMIC_INDEX = "_word_idx"
	PUNC_RUNES  = ",.?;:!()&'\""
)

/* replacePunc returns space if rune is punctuation */
func replacePunc(r rune) rune {
	rval := r
	if strings.ContainsRune(PUNC_RUNES, r) {
		rval = ' '
	}
	return rval
}

var possessivePattern = regexp.MustCompile("'s\\s")

var datePattern = regexp.MustCompile("^\\s*([0-9]{4})-([0-9]{2})\\s*$")
var moneyPattern = regexp.MustCompile("^\\s*\\$?\\s*([0-9]+)(.([0-9]{2}))?\\s*$")

const (
	POOR      = "PR"
	FAIR      = "FR"
	GOOD      = "GD"
	VERY_GOOD = "VG"
	FINE      = "FN"
	VERY_FINE = "VF"
	NEAR_MINT = "NM"
)

var gradeRank = map[string]int{POOR: 0, FAIR: 1, GOOD: 2, VERY_GOOD: 3, FINE: 4, VERY_FINE: 5, NEAR_MINT: 6}

var FRAC_NUMS = map[rune]float32{
	'¼': float32(1 / 4),
	'½': float32(1 / 2),
	'¾': float32(3 / 4),
	'⅐': float32(1 / 7),
	'⅑': float32(1 / 9),
	'⅒': float32(1 / 10),
	'⅓': float32(1 / 3),
	'⅔': float32(2 / 3),
	'⅕': float32(1 / 5),
	'⅖': float32(2 / 5),
	'⅗': float32(3 / 5),
	'⅘': float32(4 / 5),
	'⅙': float32(1 / 6),
	'⅚': float32(5 / 6),
	'⅛': float32(1 / 8),
	'⅜': float32(3 / 8),
	'⅝': float32(5 / 8),
	'⅞': float32(7 / 8),
}

var SAFE_CHARS = map[rune]bool{'-': true, '.': true, '_': true}

func init() {
	addSafeRange(SAFE_CHARS, 'a', 'z')
	addSafeRange(SAFE_CHARS, 'A', 'Z')
	addSafeRange(SAFE_CHARS, '0', '9')
}

func addSafeRange(set map[rune]bool, start, end rune) {
	for r := start; r <= end; r += 1 {
		set[r] = true
	}
}

/*
Physical copy of comic book
*/
type Book struct {
	Grade  string
	Value  int
	Signed bool
}

func (b *Book) String() string {
	return b.Grade
}

func (b *Book) FormatValue() string {
	return FormatCurrency(b.Value)
}

/*
Single issue of comic with unique cover
*/
type Comic struct {
	CoverPath   string
	Year        int
	Month       int
	Publisher   string
	SeriesId    string
	Title       string
	Subtitle    string
	Issue       string
	CoverId     string
	CoverPrice  int
	ChronOffset int
	Author      string
	CoverArtist string
	Pencils     string
	Inks        string
	Colors      string
	Letters     string
	Notes       string
	Books       []Book
}

/*
createKey creats fully qualified database key for comic
*/
func (comic *Comic) CreateKey() (key [][]byte) {
	key = append(key, []byte(comic.SeriesKey()))
	key = append(key, []byte(comic.IssueKey()))
	key = append(key, []byte(comic.CoverKey()))
	return
}

func (comic *Comic) FormatDate() string {
	return fmt.Sprintf("%d-%02d", comic.Year, comic.Month)
}

func (comic *Comic) FormatCoverPrice() string {
	return FormatCurrency(comic.CoverPrice)
}

func (comic *Comic) FormatIssue() string {
	/* ints are padded to sort lexigraphically */
	rval := comic.Issue
	for len(rval) > 1 && rval[0] == '0' {
		rval = rval[1:]
	}
	return rval
}

func (comic *Comic) SeriesKey() string {
	return SanitizeKey(comic.SeriesId)
}

func (comic *Comic) SeriesPath() string {
	return url.QueryEscape(comic.SeriesKey())
}

func (comic *Comic) IssueKey() string {
	return SanitizeKey(comic.Issue)
}

func (comic *Comic) IssuePath() string {
	return comic.SeriesPath() + "/" + url.QueryEscape(comic.IssueKey())
}

func (comic *Comic) CoverKey() string {
	return SanitizeKey(comic.CoverId)
}

func (comic *Comic) FullPath() string {
	return comic.IssuePath() + "/" + url.QueryEscape(comic.CoverKey())
}

func SanitizeKey(s string) string {
	var buffer bytes.Buffer

	s = strings.ToLower(s)
	prevUnderscore := false
	for i, w := 0, 0; i < len(s); i += w {
		r, width := utf8.DecodeRuneInString(s[i:])
		_, safe := SAFE_CHARS[r]
		if !safe && width == 1 {
			if !prevUnderscore {
				buffer.WriteRune('_')
				prevUnderscore = true
			}
		} else {
			if r == '_' && !prevUnderscore {
				buffer.WriteRune('_')
				prevUnderscore = true
			} else {
				buffer.WriteRune(r)
				prevUnderscore = false
			}
		}
		w = width
	}
	return buffer.String()
}

func UnderscoreDecode(s string) string {
	if strings.ContainsRune(s, '_') {
		var buffer bytes.Buffer
		for i, w := 0, 0; i < len(s); i += w {
			r, width := utf8.DecodeRuneInString(s[i:])
			if r == '_' {
				peek, peekWidth := readAhead(s, i+width, 2)
				pass := true
				if len(peek) == 2 {
					b, err := hex.DecodeString(peek)
					if err == nil {
						pass = false
						buffer.Write(b)
					}
				}
				if pass {
					buffer.WriteRune(r)
					buffer.WriteString(peek)
				}
				width += peekWidth
			} else {
				buffer.WriteRune(r)
			}
			w = width
		}
		return buffer.String()
	} else {
		return s
	}
}

func readAhead(s string, index, count int) (result string, width int) {
	var buffer bytes.Buffer
	for i := 0; i < count && index < len(s); {
		r, w := utf8.DecodeRuneInString(s[index:])
		buffer.WriteRune(r)
		index += w
		width += w
	}
	return buffer.String(), width
}

func (comic *Comic) IssueValue() (value float32) {
	issue := comic.Issue
	/* vast majority will be integers */
	value, success := stringIntToFloat(issue)
	if !success {
		value, success = parseFloat(issue)
	}
	if !success {
		value, success = stringFractionToFloat(issue)
	}
	if !success {
		/* no idea, just hash the damn thing */
		h := fnv.New32a()
		h.Write([]byte(comic.Issue))
		value = float32(h.Sum32())
	}
	return
}

func stringIntToFloat(s string) (value float32, success bool) {
	i, err := strconv.Atoi(s)
	if err == nil {
		value = float32(i)
		success = true
	}
	return
}

func parseFloat(s string) (value float32, success bool) {
	d, err := strconv.ParseFloat(s, 32)
	if err == nil {
		value = float32(d)
		success = true
	}
	return
}

func stringFractionToFloat(s string) (value float32, success bool) {
	/* unicode fraction? */
	ch, _ := utf8.DecodeRuneInString(s)
	v, found := FRAC_NUMS[ch]
	if found {
		value = v
		success = true
	} else {
		/* could be a multi character fraction */
		r := new(big.Rat)
		r, _ = r.SetString(s)
		if r != nil {
			value, _ = r.Float32()
			success = true
		}
	}
	return
}

/*
Best returns the physical copy that is is the best condition
*/
func (comic *Comic) Best() *Book {
	var rval *Book
	bookCount := len(comic.Books)
	if bookCount == 1 {
		rval = &comic.Books[0]
	} else if bookCount > 1 {
		bestRank := -1
		for i := range comic.Books {
			rank, ok := gradeRank[comic.Books[i].Grade]
			if ok && rank > bestRank {
				rval = &comic.Books[i]
				bestRank = rank
			}
		}
	}
	return rval
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
}

/*
ComicUpload creates a new ComicUploadHandler
*/
func ComicUpload(db *bolt.DB, webroot string) *Wrapper {

	block := CreateTemplate(webroot, "base.html", "block.template")
	login := CreateTemplate(webroot, "base.html", "login.template")
	upload := CreateTemplate(webroot, "base.html", "comicupload.template")
	ds := boltq.DataStore{db}
	return &Wrapper{ComicUploadHandler{login, block, upload, ds, webroot}}
}

var _stopWords map[string]bool

/*
getStopWords gets a list of words that should be ignored by the search index
*/
func getStopWords(tx *bolt.Tx) map[string]bool {
	if _stopWords != nil {
		return _stopWords
	} else {
		rval := make(map[string]bool)
		var words []string
		var err error
		b := tx.Bucket([]byte("text-index"))
		if b != nil {
			encoded := b.Get([]byte("stop-words"))
			if encoded != nil {
				err = json.Unmarshal(encoded, &words)
			}
		}
		if err != nil {
			log.Printf("Problem reading stop words from db: %v", err)
		} else {
			for _, word := range words {
				rval[word] = true
			}
			_stopWords = rval
		}
		return rval
	}
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
			status := processUpload(h.ds, h.webroot, r, data)
			data["Status"] = status
		}
		templateErr = h.uploadTemplate.Execute(w, data)
	}

	if templateErr != nil {
		log.Printf("Problem rendering %v\n", templateErr)
	}

	return err
}

/*
process reads the new comic data from the request and stores it in the db
*/
func processUpload(ds boltq.DataStore, webroot string, r *http.Request, data PageData) string {
	var status string
	var comic Comic
	var err error

	comic.SeriesId, status = processString(r, "seriesId", status, data)
	comic.Issue, status = processString(r, "issue", status, data)
	comic.CoverId, status = processString(r, "coverId", status, data)

	key := comic.CreateKey()
	/* TODO this isn't safe for concurrent updates */
	existing, found, err := getComic(ds, key)
	if err != nil {
		status = fmt.Sprintf("Can't lookup comic: %v", err.Error())
		/* TODO keep going? */
	} else if found {
		comic = existing
	}

	comic.Publisher, status = processString(r, "publisher", status, data)
	comic.Title, status = processString(r, "title", status, data)
	comic.ChronOffset, status = processInt(r, "chronOffset", status, data)
	comic.Year, comic.Month, status = processDate(r, "date", status, data)
	comic.Subtitle, status = processString(r, "subtitle", status, data)
	comic.CoverPrice, status = processMoney(r, "coverPrice", status, data)
	comic.Author, status = processString(r, "author", status, data)
	comic.CoverArtist, status = processString(r, "coverArtist", status, data)
	comic.Pencils, status = processString(r, "pencils", status, data)
	comic.Inks, status = processString(r, "inks", status, data)
	comic.Colors, status = processString(r, "colors", status, data)
	comic.Letters, status = processString(r, "letters", status, data)
	comic.Notes, status = processString(r, "notes", status, data)
	comic.CoverPath, status = processCover(webroot, r, &comic)
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
			err = storeComic(ds, key, &comic)
			if err != nil {
				status = fmt.Sprintf("Unable to save comic: %v", err.Error())
			} else {
				missingErr := UpdateMissingIndex(ds, comic)
				if missingErr != nil {
					log.Printf("Problem updating missing index %v", missingErr)
				}
				totalsErr := UpdateComicTotals(ds, comic.SeriesId)
				if totalsErr != nil {
					log.Printf("Problem updating comic totals %v", totalsErr)
				}
			}
		}
	}

	if status == "" {
		status = "comic uploaded successfully"
	}

	return status
}

/*
getComic returns the comic in the db matching the key.
if no such comic exists in the db, found will be false
*/
func getComic(ds boltq.DataStore, key [][]byte) (comic Comic, found bool, err error) {
	found = false
	terms := boltq.EqAll(key)
	query := boltq.NewQuery([]byte(COMIC_COL), terms...)
	err = ds.View(func(tx *bolt.Tx) error {
		encoded, e := boltq.TxQuery(tx, query)
		if encoded != nil && e == nil {
			/* TODO report if dups found */
			found = true
			e = json.Unmarshal(encoded[0], &comic)
		}
		return e
	})
	return
}

/*
storeComic stores the provided comic in the db usig the provided key
*/
func storeComic(ds boltq.DataStore, key [][]byte, comic *Comic) error {
	encoded, err := json.Marshal(comic)
	if err == nil {
		err = ds.Store([]byte(COMIC_COL), key, encoded)
	}

	if err == nil {
		err = IndexComic(ds, key, comic)
	}

	return err
}

func IndexComic(ds boltq.DataStore, key [][]byte, comic *Comic) (err error) {
	err = ds.Update(func(tx *bolt.Tx) error {
		return TxIndexComic(tx, key, comic)
	})

	return
}

func TxIndexComic(tx *bolt.Tx, key [][]byte, comic *Comic) (err error) {
	err = indexString(tx, comic.Title, key, err)
	err = indexString(tx, comic.Subtitle, key, err)
	err = indexString(tx, comic.Author, key, err)
	err = indexString(tx, comic.CoverArtist, key, err)
	err = indexString(tx, comic.Pencils, key, err)
	err = indexString(tx, comic.Inks, key, err)
	err = indexString(tx, comic.Colors, key, err)
	err = indexString(tx, comic.Letters, key, err)
	err = indexString(tx, comic.Notes, key, err)
	return
}

/*
indexString breaks up the string into fields and uses it to populate a reverse index
*/
func indexString(tx *bolt.Tx, str string, key [][]byte, err error) error {
	tokens := normalizeIndexTokens(tx, str)
	col := []byte(COMIC_COL)
	idx := []byte(COMIC_INDEX)
	for i := 0; err == nil && i < len(tokens); i += 1 {
		err = boltq.TxIndex(tx, col, idx, tokens[i], key)
	}
	return err
}

/*
splits and normalizes index token strings, also removes stop words
*/
func normalizeIndexTokens(tx *bolt.Tx, str string) (tokens [][]byte) {
	str = possessivePattern.ReplaceAllString(str, " ")
	str = strings.Map(replacePunc, str)
	parts := strings.Fields(str)
	tokens = make([][]byte, 0, len(parts))
	stopWords := getStopWords(tx)
	for i := 0; i < len(parts); i += 1 {
		lower := strings.ToLower(parts[i])
		_, isStopWord := stopWords[lower]
		if !isStopWord {
			tokens = append(tokens, []byte(lower))
		}
	}
	return
}

/*
processCover reads in the cover image from the request and stores in on the file system
*/
func processCover(webroot string, r *http.Request, comic *Comic) (coverPath, status string) {
	formFile, headers, err := r.FormFile("cover")
	if err != nil {
		if err == http.ErrMissingFile {
			if comic.CoverPath == "" {
				status = "Missing cover file"
			} else {
				coverPath = comic.CoverPath
			}
		} else {
			status = fmt.Sprintf("Unable to save cover: %v", err.Error())
		}
		return
	}
	dotIndex := strings.LastIndex(headers.Filename, ".")
	ext := headers.Filename[dotIndex:]
	dirName := comic.SeriesKey()
	issuePart := comic.IssueKey()
	coverPart := comic.CoverKey()
	fileName := fmt.Sprintf("%v_%v%v", issuePart, coverPart, ext)
	dirPath := filepath.Join("static", "comics", dirName)
	absPath := filepath.Join(webroot, dirPath)
	err = os.MkdirAll(absPath, 0700)
	if err == nil {
		cfilePath := filepath.Join(absPath, fileName)
		err = overwriteFile(cfilePath, formFile)
		coverPath = filepath.Join(dirName, fileName)
	}
	if err != nil {
		status = err.Error()
	}
	return
}

/*
overwriteFile writes the file to the path on the filesystem
*/
func overwriteFile(path string, src multipart.File) error {

	var err error
	var target *os.File
	if src != nil {
		target, err = os.Create(path)
		if err == nil {
			defer target.Close()
			_, err = io.Copy(target, src)
			if err == nil {
				err = target.Sync()
			}
		}
	} else {
		err = fmt.Errorf("Missing data for file: %v", path)
	}

	return err
}

/*
processField reads a required field from the request using the callback function f. If the current
status is not empty, it will be the returned status, otherwise any error will be used as the return status.
*/
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

/*
processString is a callback function to be used with processField which gets a string value from the request
*/
func processString(r *http.Request, field, currStatus string, data PageData) (value, status string) {
	status = processField(r, field, currStatus, func(text string) (status string) {
		value = text
		data[field] = text
		return
	})
	return
}

/*
processDate is a callback function to be used with processField which gets a date value from the request
*/
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

/*
processInt is a callback function to be used with processField which gets an integer value from the request
*/
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

/*
processMoney is a callback function to be used with processField which get a monetary value from the request
*/
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
