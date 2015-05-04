package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/smtp"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	txtemplate "text/template"
	"time"

	"github.com/boltdb/bolt"
)

/*
VideoUploadHandler handles uploads to the video page
*/
type VideoUploadHandler struct {
	loginTemplate   *template.Template
	blockedTemplate *template.Template
	uploadTemplate  *template.Template
	notifyTemplate  *txtemplate.Template
	db              *bolt.DB
	webroot         string
}

/*
VideoUpload creates a new VideoUploadHandler
*/
func VideoUpload(db *bolt.DB, webroot string) *Wrapper {

	/* TODO blocked and login templates should be shared */
	/* TODO block should be generic with description passed in */
	block := CreateTemplate(webroot, "base.html", "vidblock.template")
	login := CreateTemplate(webroot, "base.html", "vidlogin.template")
	upload := CreateTemplate(webroot, "base.html", "vidupload.template")
	notifyTemplateFile := webroot + "templates/notification.template"
	notify, err := txtemplate.ParseFiles(notifyTemplateFile)
	if err != nil {
		log.Fatalf("Unable to parse notification file %v: %v\n", notifyTemplateFile, err)
	}
	return &Wrapper{VideoUploadHandler{login, block, upload, notify, db, webroot}}
}

/*
see AppHandler interface
*/
func (h VideoUploadHandler) Handle(w http.ResponseWriter, r *http.Request,

	pagedata map[string]interface{}) *AppError {

	var login *LoginInfo
	obj, ok := pagedata["Login"]
	if ok {
		login = obj.(*LoginInfo)
	} else {
		login = getLoginInfo(r)
	}

	var err *AppError
	var templateErr error

	if !login.Authenticated() {
		templateErr = h.loginTemplate.Execute(w, pagedata)
	} else if !HasRole(h.db, login.Email, "VidUploader") {
		/* TODO send code 403 forbidden */
		templateErr = h.blockedTemplate.Execute(w, pagedata)
	} else if r.Method == "POST" {
		var status string

		title := r.FormValue("title")
		if title == "" {
			status = "Title cannot be empty"
		}
		desc := r.FormValue("description")
		vname, tname, vfile, tfile, readingErr := readFiles(r)
		if readingErr == nil && status == "" {
			/* TODO this is a lame way of checking file type */
			if !strings.HasSuffix(vname, "webm") {
				status = "Video must be a webm file"
			} else if !strings.HasSuffix(tname, "jpg") {
				status = "Thumbnail must be a jpg file"
			} else {
				err = h.process(title, desc, vname, tname, vfile, tfile)
				status = "Video uploaded successfully"
			}
		} else if readingErr == http.ErrMissingFile {
			status = "Both file and thumbnail are required"
		} else if err != nil {
			err = &AppError{readingErr, "Unable to read file", http.StatusBadRequest}
		}
		pagedata["Status"] = status
	}

	if err == nil {
		templateErr = h.uploadTemplate.Execute(w, pagedata)
		if templateErr != nil {
			log.Printf("Problem rendering %v\n", templateErr)
		}
	}

	return err
}

/*
readFiles reads the video and thumbnail files from the request
returns video and thumbnail filenames along with video and thumbnail file objects
returns http.ErrMissingFile if either are missing
*/
func readFiles(r *http.Request) (vname, tname string, vfile, tfile multipart.File, err error) {

	/*  FIXME dreamhost doesn't allow big uploads
	    workaround is to scp files and then put the filename in the form
	*/
	/*
		var headers *multipart.FileHeader
		vfile, headers, err = r.FormFile("file")
		if err == nil {
			vname = headers.Filename
			tfile, headers, err = r.FormFile("thumbnail")
			if err == nil {
				tname = headers.Filename
			}
		}
	*/

	vname = r.FormValue("file")
	tname = r.FormValue("thumbnail")

	return
}

/*
process saves the video and metadata in the filesystem and database
*/
func (h VideoUploadHandler) process(title, desc, vname, tname string,
	vfile, tfile multipart.File) *AppError {

	var appErr *AppError
	var vpath, tpath string
	var id string
	/* TODO status reporting */
	err := h.db.Update(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte("videos"))
		if b != nil {
			/* TODO way to specify author*/
			target := b.Bucket([]byte("Brian"))
			if target != nil {
				id, err = getTargetId(target)
				if err == nil {
					vpath, tpath, err = h.saveFiles(vname, tname, vfile, tfile)
					if err == nil {
						/* TODO what happens if two of these get the same id? */
						err = saveMetadata(target, id, title, desc, vpath, tpath)
					}
				}
			} else {
				log.Printf("No bucket named Brian\n")
			}
		}

		if err != nil {
			tx.Rollback()
			h.deleteFile(vpath)
			h.deleteFile(tpath)
		}
		return err
	})

	if err == nil && id != "" {
		h.sendNotification(title, "Brian", id)
	} else {
		err = fmt.Errorf("Unable to import video: %v", err)
		appErr = &AppError{err, "Internal Server Error", http.StatusInternalServerError}
	}

	return appErr
}

/*
getTargetId returns the next key to be used in the bucket
*/
func getTargetId(b *bolt.Bucket) (string, error) {

	/* TODO get rid of this mess when we switch to boltQL */
	var rval string
	var err error
	c := b.Cursor()
	lastKeyRaw, _ := c.Last()
	if lastKeyRaw != nil {
		lastKey := string(lastKeyRaw)
		keyLen := len(lastKey)
		var i int
		i, err = strconv.Atoi(lastKey)
		if err == nil {
			i += 1
			numstr := strconv.Itoa(i)
			newLen := len(numstr)
			padLen := keyLen - newLen
			if padLen > 0 {
				rval = strings.Repeat("0", padLen)
			} else {
				rval = numstr
			}
			/* super lame, need a better key system */
			if len(rval) != keyLen {
				err = fmt.Errorf("Key overflow: %v doesn't have %v digits", i, keyLen)
			}
		}
	} else {
		/* TODO better default */
		rval = "000"
	}

	return rval, err
}

/*
saveFiles saves the video and thumbnail files to the filesystem
returns the relative paths to the files for use in the video metadata
*/
func (h VideoUploadHandler) saveFiles(vname, tname string, vfile, tfile multipart.File) (vpath,
	tpath string, err error) {

	now := time.Now()
	/* TODO hard coded data directory */
	dirName := fmt.Sprintf("data-brian%v", now.Year())
	dirPath := filepath.Join(h.webroot, "videos", dirName)
	err = os.MkdirAll(dirPath, 0700)
	if err == nil {
		/* TODO handle name conflict */
		vpath = filepath.Join(dirName, vname)
		vabs := filepath.Join(dirPath, vname)
		err = writeFile(vabs, vfile)
		if err == nil {
			tpath = filepath.Join(dirName, tname)
			tabs := filepath.Join(dirPath, tname)
			err = writeFile(tabs, tfile)
		}
	}
	return
}

/*
writeFile writes the file to the path on the filesystem
*/
func writeFile(path string, src multipart.File) error {

	var target *os.File
	_, err := os.Stat(path)
	/* no op if file already exists */
	/* TODO that could bite us if names are sloppy */
	if os.IsNotExist(err) {
		target, err = os.Create(path)
		if err == nil {
			defer target.Close()
			_, err = io.Copy(target, src)
			if err != nil {
				err = target.Sync()
			}
		}
	}

	return err
}

/*
deleteFile deletes the file specified by path
*/
func (h VideoUploadHandler) deleteFile(path string) {

	absPath := filepath.Join(h.webroot, "videos", path)
	err := os.Remove(absPath)
	if err != nil {
		log.Printf("Unable to cleanup file %v. %v\n", absPath, err)
	}
}

/*
saveMetadata creates a video metadata object and saves it in the provided bucket at id
*/
func saveMetadata(b *bolt.Bucket, id, title, desc, vpath, tpath string) error {

	/* TODO hard coded type */
	vidFile := VidFile{vpath, "video/webm"}
	vid := Video{desc, tpath, title, []VidFile{vidFile}}
	encoded, err := json.Marshal(vid)
	if err == nil {
		err = b.Put([]byte(id), encoded)
	}
	return err
}

/*
sendNotification sends out email notification to all video subscribers
*/
func (h VideoUploadHandler) sendNotification(title, data, id string) {

	subscribers, err := UsersWithRole(h.db, "VidSubscriber")
	if err == nil && len(subscribers) > 0 {
		creds, found := GetMailCreds(h.db)
		if found {
			msgData := map[string]string{
				"From":  creds.From,
				"Data":  data,
				"Video": id,
				"Title": title,
			}
			auth := smtp.PlainAuth("", creds.From, creds.Password, creds.Host)
			var msg bytes.Buffer
			err = h.notifyTemplate.Execute(&msg, msgData)
			if err == nil {
				qualified := fmt.Sprintf("%v:%d", creds.Host, creds.Port)
				err = smtp.SendMail(qualified, auth, creds.From, subscribers, msg.Bytes())
			}
		}
	}

	if err != nil {
		log.Printf("Unable to send notifications: %v\n", err)
	}
}
