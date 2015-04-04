package handler

import (
    "bufio"
    "github.com/bclement/textgen"
	"html/template"
	"net/http"
    "io/ioutil"
    "log"
    "os"
    "path"
)

type MissingHandler struct {
    initerr error
    template *template.Template
    generator *textgen.Generator
}

func Missing(webroot string, t *template.Template) *MissingHandler {
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

func (h MissingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    login := getLoginInfo(r)
    pagedata := map[string]interface{}{"Login":login}

	headers := w.Header()
	headers.Add("Content-Type", "text/html")

    text, err := h.generator.GenerateString(512)
    if err != nil || len(text) == 0 {
        log.Printf("Unable to generate text: %v\n", err)
        text = "I can't think of anything at the moment..."
    }
    pagedata["Text"] = text

	h.template.Execute(w, pagedata)
}

