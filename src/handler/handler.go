package handler

import (
	"html/template"
    "io"
    "log"
    "mime"
    "net/http"
    "os"
    "path/filepath"
)

type HandlerFunc func(http.ResponseWriter, *http.Request) *AppError

func (hf HandlerFunc) Handle(w http.ResponseWriter, r *http.Request) *AppError {
    return hf(w, r)
}

type AppHandler interface {
    /* handles requests for a given page */
    Handle(http.ResponseWriter, *http.Request) *AppError
}

/* wrapper takes care of error handling */
type Wrapper struct {
    Handler AppHandler
}

type AppError struct {
    Err error
    Message string
    Code int
}

/* serveHTTP formats and passes up an error */
func (hw Wrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if appErr := hw.Handler.Handle(w, r); appErr != nil {
        if appErr.Err != nil {
            log.Println(appErr.Err)
        }
        http.Error(w, appErr.Message, appErr.Code)
    }
}

type PageData struct {
    Login *LoginInfo
}

type GenericHandler struct {
    Template *template.Template
}

func (h GenericHandler) Handle(w http.ResponseWriter, r *http.Request) *AppError {
    login := getLoginInfo(r)

	headers := w.Header()
	headers.Add("Content-Type", "text/html")

	h.Template.Execute(w, PageData{login})
    return nil
}

/* copies file denoted by fname to response */
func ServeFile(w http.ResponseWriter, fname string) *AppError {
	/* FIXME ensure path is sanitary */
	f, err := os.Open(fname)
	if err != nil {
		if os.IsNotExist(err) {
            return &AppError{err, "File Not Found", http.StatusNotFound}
		} else {
            return &AppError{err, "Internal Server Error", http.StatusInternalServerError}
		}
	} else {
		WriteContentHeader(w, fname)
		io.Copy(w, f)
	}
    return nil
}

/* writes header for content type of file denoted by fname if it can be determined */
func WriteContentHeader(w http.ResponseWriter, fname string) {
	extension := filepath.Ext(fname)
	if extension != "" {
		mimetype := mime.TypeByExtension(extension)
		if mimetype != "" {
			headers := w.Header()
			headers.Add("Content-Type", mimetype)
		}
	}
}
