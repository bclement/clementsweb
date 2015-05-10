package handler

import (
	"fmt"
	"html/template"
	"log"
	"net/http"

	"github.com/boltdb/bolt"
)

/*
AdminHandler handles site administration
*/
type AdminHandler struct {
	loginTemplate   *template.Template
	blockedTemplate *template.Template
	adminTemplate   *template.Template
	db              *bolt.DB
	webroot         string
}

/*
Admin creates a new AdminHandler
*/
func Admin(db *bolt.DB, webroot string) *Wrapper {

	/* TODO blocked and login templates should be shared */
	/* TODO block should be generic with description passed in */
	block := CreateTemplate(webroot, "base.html", "block.template")
	login := CreateTemplate(webroot, "base.html", "login.template")
	admin := CreateTemplate(webroot, "base.html", "admin.template")
	return &Wrapper{AdminHandler{login, block, admin, db, webroot}}
}

/*
see AppHandler interface
*/
func (h AdminHandler) Handle(w http.ResponseWriter, r *http.Request,
	pagedata map[string]interface{}) *AppError {

	var err *AppError
	var status string

	authorized, templateErr := handleAuth(w, r, h.loginTemplate, h.blockedTemplate,
		h.db, pagedata, "Admin", "")
	if authorized && templateErr == nil {
		if r.Method == "POST" {
			action := r.FormValue("action")
			user := r.FormValue("user")
			role := r.FormValue("role")
			status = h.populateInfo(pagedata)
			if user == "" || role == "" {
				status = "User and role are required"
			} else {
				var opErr error
				if action == "Add" {
					_, _, opErr = AddRole(h.db, user, role, true)
				} else if action == "Remove" {
					_, _, opErr = RemoveRole(h.db, user, role)
				} else {
					status = "Unknown action: " + action
				}
				if opErr != nil {
					status = fmt.Sprintf("Unable to %v: %v", action, opErr)
				} else {
					status = h.populateInfo(pagedata)
				}
			}
		} else {
			status = h.populateInfo(pagedata)
		}

		pagedata["Status"] = status
		templateErr = h.adminTemplate.Execute(w, pagedata)
	}
	if templateErr != nil {
		log.Printf("Problem rendering %v\n", templateErr)
	}

	return err
}

/*
populateInfo populates the page data with all the user info objects in the DB
returns any error as status string or empty string if no error
*/
func (h AdminHandler) populateInfo(pagedata map[string]interface{}) string {
	status := ""
	infos, opErr := GetAllUsers(h.db)
	if opErr == nil {
		pagedata["Infos"] = infos
	} else {
		status = fmt.Sprintf("Can't list roles: %v", opErr)
	}
	return status
}
