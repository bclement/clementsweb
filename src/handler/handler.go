package handler

import (
	"html/template"
    "log"
    "mime"
    "net/http"
    "path/filepath"
)

type HandlerFunc func(http.ResponseWriter, *http.Request) *AppError

func (hf HandlerFunc) Handle(w http.ResponseWriter, r *http.Request,
        pagedata map[string]interface{} ) *AppError {
    return hf(w, r)
}

type AppHandler interface {
    /* handles requests for a given page */
    Handle(http.ResponseWriter, *http.Request, map[string]interface{}) *AppError
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
    login := getLoginInfo(r)
    pagedata := map[string]interface{}{"Login":login}
    pagedata["PathInfo"] = r.URL.String()
    if appErr := hw.Handler.Handle(w, r, pagedata); appErr != nil {
        if appErr.Err != nil {
            log.Println(appErr.Err)
        }
        http.Error(w, appErr.Message, appErr.Code)
    }
}

type GenericHandler struct {
    Template *template.Template
}

func (h GenericHandler) Handle(w http.ResponseWriter, r *http.Request,
        data map[string]interface{}) *AppError {
	headers := w.Header()
	headers.Add("Content-Type", "text/html")
    log.Printf("data: %v\n", data)

	h.Template.Execute(w, data)
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
