package handler

import (
	"encoding/json"
    "fmt"
	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
    "github.com/stretchr/gomniauth"
    "github.com/stretchr/gomniauth/providers/google"
    "github.com/stretchr/objx"
    "github.com/stretchr/signature"
    "io"
    "net/http"
)

const (
    GoogleProviderId = "google"
    GoogleCallbackPath = "/auth/" + GoogleProviderId + "/callback"
    GoogleLoginPath = "/auth/" + GoogleProviderId + "/login"
)

type ProviderInfo struct {
    Clientid string
    Clientsecret string
}

func getInfo(db *bolt.DB, provider string) (ProviderInfo, error) {
    var rval ProviderInfo
    err := db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket([]byte("auth.providers"))
        if b != nil {
            encoded := b.Get([]byte(provider))
            if encoded != nil {
                err := json.Unmarshal(encoded, &rval)
                if err != nil {
                    return err
                }
            }
        }
        return nil
    })
    return rval, err
}

func RegisterAuth(db *bolt.DB, r *mux.Router, baseUrl string) {
    gomniauth.SetSecurityKey(signature.RandomKey(64))
    googleInfo, _ := getInfo(db, GoogleProviderId)
    googleCallbackUrl := baseUrl + GoogleCallbackPath
    gomniauth.WithProviders(google.New(googleInfo.Clientid, googleInfo.Clientsecret, googleCallbackUrl))
    r.Handle(GoogleLoginPath, loginHandler(GoogleProviderId))
    r.Handle(GoogleCallbackPath, callBackHandler(GoogleProviderId))
}

func loginHandler(providerName string) Wrapper {
    provider, err := gomniauth.Provider(providerName)
    if err != nil {
        panic(err)
    }
    f := func(w http.ResponseWriter, r *http.Request) *AppError {
        state := gomniauth.NewState("after", "success")
        authUrl, err := provider.GetBeginAuthURL(state, nil)
        if err != nil {
            return &AppError{err, "Unable to get auth URL after success", http.StatusInternalServerError}
        }
        // redirect
        http.Redirect(w, r, authUrl, http.StatusFound)
        return nil
    }
    return Wrapper{HandlerFunc(f)}
}

func callBackHandler(providerName string) Wrapper {
    provider, err := gomniauth.Provider(providerName)
    if err != nil {
        panic(err)
    }
    f := func(w http.ResponseWriter, r *http.Request) *AppError {
        omap, err := objx.FromURLQuery(r.URL.RawQuery)
        if err != nil {
            return &AppError{err, "Unable to get query params", http.StatusInternalServerError}
        }
        creds, err := provider.CompleteAuth(omap)
        if err != nil {
            return &AppError{err, "Unable to get credentials", http.StatusInternalServerError}
        }
        // load the user
        user, userErr := provider.GetUser(creds)
        if userErr != nil {
            return &AppError{err, "Unable to get user", http.StatusInternalServerError}
        }
        data := fmt.Sprintf("%#v", user)
        io.WriteString(w, data)
        return nil
    }
    return Wrapper{HandlerFunc(f)}
}
