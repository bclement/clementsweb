package handler

import (
	"html/template"
	"log"
	"net/http"
)

type PageData map[string]interface{}

/*
HandlerFunc is a function type that fulfills the AppHandler interface
*/
type HandlerFunc func(http.ResponseWriter, *http.Request) *AppError

/*
see AppHandler interface
*/
func (hf HandlerFunc) Handle(w http.ResponseWriter, r *http.Request,
	data PageData) *AppError {
	return hf(w, r)
}

type AppHandler interface {
	/*
	   handles requests for a given page
	   the map is a preloaded page data map that includes user login information
	*/
	Handle(http.ResponseWriter, *http.Request, PageData) *AppError
}

/*
Wrapper takes care of common handler tasks such as login lookup and error handling
*/
type Wrapper struct {
	Handler AppHandler
}

/*
AppError contains HTTP specific error information
*/
type AppError struct {
	Err     error
	Message string
	Code    int
}

/*
ServeHTTP takes care of common handler tasks such as error handling
*/
func (hw Wrapper) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	login := getLoginInfo(r)
	data := PageData{"Login": login}
	data["PathInfo"] = r.URL.String()
	if appErr := hw.Handler.Handle(w, r, data); appErr != nil {
		if appErr.Err != nil {
			log.Println(appErr.Err)
		}
		http.Error(w, appErr.Message, appErr.Code)
	}
}

/*
GenericHandler is useful for pages which only have login info as dynamic conten
*/
type GenericHandler struct {
	Template *template.Template
}

/*
see AppHandler interface
*/
func (h GenericHandler) Handle(w http.ResponseWriter, r *http.Request,
	data PageData) *AppError {
	headers := w.Header()
	headers.Add("Content-Type", "text/html")

	h.Template.Execute(w, data)
	return nil
}

/*
CreateTemplate parses template files into HTTP template objects
*/
func CreateTemplate(webroot string, filenames ...string) *template.Template {
	files := make([]string, len(filenames))
	for i, f := range filenames {
		files[i] = webroot + "templates/" + f
	}
	rval, err := template.ParseFiles(files...)
	if err != nil {
		log.Fatalf("Unable to parse files: %v. %v", files, err)
	}
	return rval
}
