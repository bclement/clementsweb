package handler

import (
	"net/http"
)

/*
RedirectHandler redirects requests to another page
*/
type RedirectHandler struct {
	target string
}

/*
Redirect creates a new RedirectHandler
*/
func Redirect(target string) *Wrapper {
	return &Wrapper{RedirectHandler{target}}
}

/*
see AppHandler interface
*/
func (h RedirectHandler) Handle(w http.ResponseWriter, r *http.Request,
	data PageData) *AppError {
	http.Redirect(w, r, h.target, 301)
	return nil
}
