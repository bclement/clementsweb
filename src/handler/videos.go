package handler

import (
    "encoding/json"
    "fmt"
    "github.com/boltdb/bolt"
	"github.com/gorilla/mux"
    "html/template"
    "net/http"
)

type VideoHandler struct {
    playerTemplate *template.Template
    listTemplate *template.Template
    db *bolt.DB
    webroot string
}

func Videos(db *bolt.DB, pt, lt *template.Template, webroot string) *Wrapper {
    return &Wrapper{VideoHandler{pt, lt, db, webroot}}
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

func (h VideoHandler) Handle(w http.ResponseWriter, r *http.Request) *AppError {
	vars := mux.Vars(r)
    path, present := vars["path"]
    if present {
        resourcePath := h.webroot + "/videos/" + path
        return ServeFile(w, resourcePath)
    }
    var err *AppError
    login := getLoginInfo(r)

    pagedata := map[string]interface{}{"Login":login}

    headers := w.Header()
    headers.Add("Content-Type", "text/html")

    data := r.FormValue("d")
    vidID := r.FormValue("v")
    if vidID == "" {
        keys, vids, err := h.listVideos(data)
        if err == nil {
            fmt.Printf("%v\n", vids)
            pagedata["Videos"] = vids
            pagedata["Creators"] = keys
            if data == "" && len(keys) > 0 {
                pagedata["Data"] = keys[0]
            }else {
                pagedata["Data"] = data
            }
            h.listTemplate.Execute(w, pagedata)
        }
    } else {
        video, err := h.getVideo(data, vidID)
        if err == nil {
            pagedata["Video"] = video
            h.playerTemplate.Execute(w, pagedata)
        }
    }
    return err
}

func (h VideoHandler) listVideos(data string) ([]string, map[string]Video, *AppError) {
    var keys []string
    var vids map[string]Video
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

            fmt.Println(datakey)
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
            rval = append(rval, string(v))
        }
    }
    return rval
}

func listVideos(bucket *bolt.Bucket) (map[string]Video, error) {
    rval := make(map[string]Video)
    var err error
    cur := bucket.Cursor()
    fmt.Printf("here\n")
    for k, v := cur.First(); k != nil; k, v = cur.Next() {
        fmt.Printf("k %v %v\n", string(k), v == nil)
        if v != nil {
            var vid Video
            err := json.Unmarshal(v, &vid)
            if err != nil {
                break
            }
            rval[string(k)] = vid
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
