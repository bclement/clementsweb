package handler

import (
    "encoding/json"
    "github.com/boltdb/bolt"
    "github.com/gorilla/mux"
    "github.com/gorilla/securecookie"
    "github.com/gorilla/sessions"
    "github.com/stretchr/gomniauth"
    "github.com/stretchr/gomniauth/providers/google"
    "github.com/stretchr/objx"
    "github.com/stretchr/signature"
    "log"
    "net/http"
)

const (
    GoogleProviderId = "google"
    GoogleCallbackPath = "/auth/" + GoogleProviderId + "/callback"
    GoogleLoginPath = "/auth/" + GoogleProviderId + "/login"

    LogoutPath = "/auth/logout"

    SessionName = "clementscode-session"
    EmailKey = "email"
    NameKey = "name"
)

var store = sessions.NewCookieStore(securecookie.GenerateRandomKey(64))

type LoginInfo struct {
    Email string
    Name string
}

func (li *LoginInfo) Authenticated() bool {
    return li.Email != ""
}

type ProviderInfo struct {
    Clientid string
    Clientsecret string
}

type UserInfo struct {
    Email string
    Roles []string
}

func getProviderInfo(db *bolt.DB, provider string) (ProviderInfo, error) {
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

func GetUserInfo(db *bolt.DB, email string) (UserInfo, bool, error) {
    var rval UserInfo
    found := false
    err := db.View(func(tx *bolt.Tx) error {
        b := tx.Bucket([]byte("auth.userinfo"))
        if b != nil {
            encoded := b.Get([]byte(email))
            if encoded != nil {
                found = true
                err := json.Unmarshal(encoded, &rval)
                if err != nil {
                    return err
                }
            }
        }
        return nil
    })
    return rval, found, err
}

func HasRole(db *bolt.DB, email, role string) bool {
    rval := false
    info, found, err := GetUserInfo(db, email)
    if found {
        for _, userRole := range info.Roles {
            if role == userRole {
                rval = true
                break
            }
        }
    }
    if err != nil {
        log.Printf("Problem checking roles %v\n", err)
    }
    return rval
}

func RegisterAuth(useOAuth bool, db *bolt.DB, r *mux.Router, baseUrl string) {
    if useOAuth {
        gomniauth.SetSecurityKey(signature.RandomKey(64))
        googleInfo, _ := getProviderInfo(db, GoogleProviderId)
        googleCallbackUrl := baseUrl + GoogleCallbackPath
        goog := google.New(googleInfo.Clientid, googleInfo.Clientsecret, googleCallbackUrl)
        gomniauth.WithProviders(goog)
        r.Handle(GoogleLoginPath, loginHandler(GoogleProviderId))
        r.Handle(GoogleCallbackPath, callBackHandler(GoogleProviderId))
    } else {
        r.Handle(GoogleLoginPath, Wrapper{HandlerFunc(handleTestAuth)})
    }
    logout := Wrapper{HandlerFunc(handleLogout)}
    r.Handle(LogoutPath, logout)
}

func handleTestAuth(w http.ResponseWriter, r *http.Request) *AppError {
    session, _ := store.Get(r, SessionName)
    session.Values[EmailKey] = "test@example.com"
    session.Values[NameKey] = "Test User"

    session.Save(r, w)
    /* TODO redirect to last page user was on */
    http.Redirect(w, r, "/", http.StatusFound)
    return nil
}

func getLoginInfo(r *http.Request) *LoginInfo {
    session, _ := store.Get(r, SessionName)
    email := getSessionValue(session, EmailKey)
    name := getSessionValue(session, NameKey)
    return &LoginInfo{email, name}
}

func getSessionValue(session *sessions.Session, key string) string {
    rval , _ := session.Values[key].(string)
    return rval
}

func handleLogout(w http.ResponseWriter, r *http.Request) *AppError {
    session, _ := store.Get(r, SessionName)
    session.Values[EmailKey] = ""
    session.Values[NameKey] = ""
    /* this is the suggested way to delete a session,
    but firefox seems to ignore a cookie that is already expired */
    //session.Options = &sessions.Options{MaxAge: -1}
    session.Save(r, w)
    /* TODO redirect to last page user was on */
    http.Redirect(w, r, "/", http.StatusFound)
    return nil
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

        session, _ := store.Get(r, SessionName)
        session.Values[EmailKey] = user.Email()
        session.Values[NameKey] = user.Name()

        session.Save(r, w)
        /* TODO redirect to last page user was on */
        http.Redirect(w, r, "/", http.StatusFound)
        return nil
    }
    return Wrapper{HandlerFunc(f)}
}
