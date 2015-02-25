package handler

import (
    "io"
    "log"
    "mime"
    "net/http"
    "os"
    "path/filepath"
)

type AppHandler interface {
    /* handles requests for a given page */
    Handle(http.ResponseWriter, *http.Request) *AppError
}

/* wrapper takes care of error handling */
type HandlerWrapper struct {
    Handler AppHandler
}

type handlertype func(http.ResponseWriter, *http.Request) *AppError

func (ht handlertype) Handle(w http.ResponseWriter, r *http.Request) *AppError {
    return ht(w, r)
}

func WrapHandler(h handlertype) *HandlerWrapper {
    return &HandlerWrapper{h}
}

type AppError struct {
    Err error
    Message string
    Code int
}

/* serveHTTP formats and passes up an error */
func (hw HandlerWrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    if e := hw.Handler.Handle(w, r); e != nil { // e is *AppError, not os.Error.
        if e.Err != nil {
            log.Println(e.Err)
        }
        http.Error(w, e.Message, e.Code)
    }
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
