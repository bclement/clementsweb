package handler

import (
    "encoding/json"
    "fmt"
    "github.com/boltdb/bolt"
	"github.com/gorilla/mux"
    "html/template"
    "log"
    "net/http"
)

type VideoHandler struct {
    loginTemplate *template.Template
    blockedTemplate *template.Template
    playerTemplate *template.Template
    listTemplate *template.Template
    db *bolt.DB
    webroot string
}

func Videos(db *bolt.DB, lit, bt, pt, lt *template.Template, webroot string) *Wrapper {
    return &Wrapper{VideoHandler{lit, bt, pt, lt, db, webroot}}
}

type VidFile struct {
    Path string
    Type string
}

type Video struct {
    Description string
    Thumbnail string
    Title string
    VidFiles []VidFile
}

type VidMap struct {
    Keys []string
    Entries map[string]Video
}

func NewVidMap() *VidMap {
    k := make([]string, 0, 8)
    e := make(map[string]Video)
    return &VidMap{k, e}
}

func (vm *VidMap) Put(key string, value Video) {
    vm.Keys = append(vm.Keys, key)
    vm.Entries[key] = value
}

func (vm *VidMap) ReverseKeys() []string {
    size := len(vm.Keys)
    last := size - 1
    rval := make([]string, size)
    for i := range vm.Keys {
        rval[last - i] = vm.Keys[i]
    }
    return rval
}

func (h VideoHandler) Handle(w http.ResponseWriter, r *http.Request) *AppError {
    login := getLoginInfo(r)
    pagedata := map[string]interface{}{"Login":login}

    var err *AppError
    var templateErr error

    if !login.Authenticated() {
        templateErr = h.loginTemplate.Execute(w, pagedata)
    } else if !HasRole(h.db, login.Email, "VidWatcher") {
        /* TODO send code 403 forbidden */
        templateErr = h.blockedTemplate.Execute(w, pagedata)
    } else {
        err = h.serve(login, w, r)
    }
    if templateErr != nil {
        log.Printf("Problem rendering %v\n", templateErr)
    }

    return err
}

func (h VideoHandler) serve(login *LoginInfo, w http.ResponseWriter, r *http.Request) *AppError {
    var err *AppError
	vars := mux.Vars(r)
    path, present := vars["path"]
    if present {
        resourcePath := h.webroot + "/videos/" + path
        return ServeFile(w, resourcePath)
    }

    pagedata := map[string]interface{}{"Login":login}

    headers := w.Header()
    headers.Add("Content-Type", "text/html")

    var templateErr error
    data := r.FormValue("d")
    vidID := r.FormValue("v")
    if vidID == "" {
        keys, vids, err := h.listVideos(data)
        if err == nil {
            pagedata["Videos"] = vids
            pagedata["Creators"] = keys
            if data == "" && len(keys) > 0 {
                pagedata["Data"] = keys[0]
            }else {
                pagedata["Data"] = data
            }
            templateErr = h.listTemplate.Execute(w, pagedata)
        }
    } else {
        video, err := h.getVideo(data, vidID)
        if err == nil {
            pagedata["Video"] = video
            templateErr = h.playerTemplate.Execute(w, pagedata)
        }
    }
    if templateErr != nil {
        log.Printf("Problem rendering %v\n", templateErr)
    }
    return err
}

func (h VideoHandler) listVideos(data string) ([]string, *VidMap, *AppError) {
    var keys []string
    var vids *VidMap
    var appErr *AppError
    err := h.db.View(func(tx *bolt.Tx) error {
        var err error
        b := tx.Bucket([]byte("videos"))
        if b != nil {
            keys = listSubBuckets(b)
            var datakey string
            if data == "" && len(keys) > 0 {
                datakey = keys[0]
            } else {
                datakey = data
            }

            nested := b.Bucket([]byte(datakey))
            if nested != nil {
                vids, err = listVideos(nested)
            }
        }

        return err
    })
    if err != nil {
        err = fmt.Errorf("Unable to get videos from db: %v", err)
        appErr = &AppError{err, "Internal Server Error", http.StatusInternalServerError}
    }
    return keys, vids, appErr
}

func listSubBuckets(bucket *bolt.Bucket) []string {
    var rval []string
    cur := bucket.Cursor()
    for k, v := cur.First(); k != nil; k, v = cur.Next() {
        /* sub buckets have nil values */
        if v == nil {
            rval = append(rval, string(k))
        }
    }
    return rval
}

func listVideos(bucket *bolt.Bucket) (*VidMap, error) {
    rval := NewVidMap()
    var err error
    cur := bucket.Cursor()
    for k, v := cur.First(); k != nil; k, v = cur.Next() {
        if v != nil {
            var vid Video
            err := json.Unmarshal(v, &vid)
            if err != nil {
                break
            }
            rval.Put(string(k), vid)
        }
    }
    return rval, err
}

func (h VideoHandler) getVideo(data, vidID string) (*Video, *AppError) {
    var rval Video
    err := h.db.View(func(tx *bolt.Tx) error {
        var err error
        b := tx.Bucket([]byte("videos"))
        if b != nil {
            nested := b.Bucket([]byte(data))
            if nested != nil {
                encoded := nested.Get([]byte(vidID))
                if encoded != nil {
                    err = json.Unmarshal(encoded, &rval)
                }
            }
        }

        return err
    })
    if err != nil {
        err = fmt.Errorf("Unable to get video from db: %v", err)
        return nil, &AppError{err, "Internal Server Error", http.StatusInternalServerError}
    }
    return &rval, nil
}
