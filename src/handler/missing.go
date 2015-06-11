package handler

import (
	"bufio"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"

	"github.com/bclement/textgen"
)

/*
MissingHandler is used to generate the 404 page
*/
type MissingHandler struct {
	initerr   error
	template  *template.Template
	generator *textgen.Generator
}

/*
Missing creates a new MissingHandler
*/
func Missing(webroot string) *MissingHandler {
	t := CreateTemplate(webroot, "base.html", "missing.template")
	var filenames []string
	inputdir := path.Join(webroot, "textgeninput")
	infos, err := ioutil.ReadDir(inputdir)
	for i := 0; err == nil && i < len(infos); i += 1 {
		fname := path.Join(inputdir, infos[i].Name())
		filenames = append(filenames, fname)
	}
	gen := textgen.NewGenerator(2)
	for i := 0; err == nil && i < len(filenames); i += 1 {
		var f *os.File
		f, err := os.Open(filenames[i])
		if err == nil {
			r := bufio.NewReader(f)
			err = gen.Load(r)
		}
	}
	if err != nil {
		log.Printf("Problem initializing text gen: %v\n", err)
	}
	return &MissingHandler{err, t, gen}
}

/*
see http handler interface
*/
func (h MissingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	login := getLoginInfo(r)
	data := PageData{"Login": login}

	headers := w.Header()
	headers.Add("Content-Type", "text/html")

	text, err := h.generator.GenerateString(512)
	if err != nil || len(text) == 0 {
		log.Printf("Unable to generate text: %v\n", err)
		text = "I can't think of anything at the moment..."
	}
	data["Text"] = text

	h.template.Execute(w, data)
}
