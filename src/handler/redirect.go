package handler

import (
	"net/http"
)

type RedirectHandler struct {
    target string
}

func Redirect(target string) *Wrapper {
    return &Wrapper{RedirectHandler{target}}
}

func (h RedirectHandler) Handle(w http.ResponseWriter, r *http.Request) *AppError {
    http.Redirect(w, r, h.target, 301)
    return nil
}

