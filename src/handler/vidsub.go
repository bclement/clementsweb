package handler

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/boltdb/bolt"
)

/*
VideoSubHandler handles email subscriptions for videos
*/
type VideoSubHandler struct {
	loginTemplate   *template.Template
	blockedTemplate *template.Template
	subTemplate     *template.Template
	db              *bolt.DB
}

/*
VideoSub creates a new VideoSubHandler
*/
func VideoSub(db *bolt.DB, webroot string) *Wrapper {

	/* TODO blocked and login templates should be shared */
	/* TODO block should be generic with description passed in */
	block := CreateTemplate(webroot, "base.html", "block.template")
	login := CreateTemplate(webroot, "base.html", "login.template")
	sub := CreateTemplate(webroot, "base.html", "vidsub.template")
	return &Wrapper{VideoSubHandler{login, block, sub, db}}
}

/*
see AppHandler interface
*/
func (h VideoSubHandler) Handle(w http.ResponseWriter, r *http.Request,
	pagedata PageData) *AppError {

	var login *LoginInfo
	obj, ok := pagedata["Login"]
	if ok {
		login = obj.(*LoginInfo)
	} else {
		login = getLoginInfo(r)
	}

	var err *AppError
	var templateErr error
	var status bool

	if !login.Authenticated() {
		templateErr = h.loginTemplate.Execute(w, pagedata)
	} else if !HasRole(h.db, login.Email, "VidWatcher") {
		/* TODO send code 403 forbidden */
		templateErr = h.blockedTemplate.Execute(w, pagedata)
	} else if r.Method == "POST" {
		action := r.FormValue("action")
		if action == "Subscribe" {
			status, err = h.subscribe(login)
		} else if action == "Unsubscribe" {
			status, err = h.unsubscribe(login)
		} else {
			status = h.status(login)
		}
	} else {
		status = h.status(login)
	}

	if err == nil {
		pagedata["Subscribed"] = status
		templateErr = h.subTemplate.Execute(w, pagedata)
		if templateErr != nil {
			log.Printf("Problem rendering %v\n", templateErr)
		}
	}

	return err
}

/*
status returns true if the user from login is subscribed to videos
*/
func (h VideoSubHandler) status(login *LoginInfo) bool {

	/* TODO may need to report if db error happens */
	return HasRole(h.db, login.Email, "VidSubscriber")
}

/*
subscribe ensures that the user is subscribed to video notifications
returns status of user subscription (should always be true)
*/
func (h VideoSubHandler) subscribe(login *LoginInfo) (bool, *AppError) {

	var appErr *AppError
	status := h.status(login)
	if !status {
		var err error
		var found bool
		_, found, err = AddRole(h.db, login.Email, "VidSubscriber", false)
		if found && err == nil {
			status = true
		} else if err != nil {
			err = fmt.Errorf("Unable to add subscription: %v", err)
			appErr = &AppError{err, "Internal Server Error", http.StatusInternalServerError}
		}
	}
	return status, appErr
}

/*
unsubscribe ensures that the user is not subscribed to video notifications
returns status of user subscription (should always be false)
*/
func (h VideoSubHandler) unsubscribe(login *LoginInfo) (bool, *AppError) {

	var appErr *AppError
	status := h.status(login)
	if status {
		var err error
		var found bool
		_, found, err = RemoveRole(h.db, login.Email, "VidSubscriber")
		if found && err == nil {
			status = false
		} else if err != nil {
			err = fmt.Errorf("Unable to remove subscription: %v", err)
			appErr = &AppError{err, "Internal Server Error", http.StatusInternalServerError}
		}
	}
	return status, appErr
}
