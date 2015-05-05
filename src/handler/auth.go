package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	"github.com/stretchr/gomniauth"
	"github.com/stretchr/gomniauth/providers/google"
	"github.com/stretchr/objx"
	"github.com/stretchr/signature"
)

const (
	GoogleProviderId   = "google"
	GoogleCallbackPath = "/auth/" + GoogleProviderId + "/callback"
	GoogleLoginPath    = "/auth/" + GoogleProviderId + "/login"

	LogoutPath = "/auth/logout"

	SessionName = "clementscode-session"
	EmailKey    = "email"
	NameKey     = "name"
)

/*
store keeps track of the secure cookies used for login sessions
*/
var store = sessions.NewCookieStore(securecookie.GenerateRandomKey(64))

/*
MailCreds stored credentials for email services
*/
type MailCreds struct {
	From     string `json:"from"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Password string `json:"password"`
}

/*
LoginInfo holds information for the currently logged in user form OAuth
*/
type LoginInfo struct {
	Email string
	Name  string
}

/*
Authenticated returns true if LoginInfo was authenticated via OAuth
*/
func (li *LoginInfo) Authenticated() bool {
	return li.Email != ""
}

/*
ProviderInfo is used to store OAuth provider info in the database
*/
type ProviderInfo struct {
	Clientid     string
	Clientsecret string
}

/*
UserInfo is used to store authorization info in the database
*/
type UserInfo struct {
	Email string
	Roles []string
}

/*
getProviderInfo fetches provider information from the database
*/
func getProviderInfo(db *bolt.DB, provider string) (ProviderInfo, error) {
	var rval ProviderInfo
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte("auth.providers"))
		if b != nil {
			encoded := b.Get([]byte(provider))
			if encoded != nil {
				err = json.Unmarshal(encoded, &rval)
			}
		}
		return err
	})
	return rval, err
}

/*
GetMailCreds gets the email server credentials.
The seond return value is only true if the object is populated and ready to read.
*/
func GetMailCreds(db *bolt.DB) (MailCreds, bool) {
	var rval MailCreds
	found := false
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte("auth.services"))
		if b != nil {
			encoded := b.Get([]byte("mail"))
			if encoded != nil {
				err = json.Unmarshal(encoded, &rval)
				found = err == nil
			}
		}
		return err
	})
	if err != nil {
		log.Printf("Problem getting mail creds %v\n", err)
	}

	return rval, found
}

/*
GetUserInfo fetches user authorization info from the database
*/
func GetUserInfo(db *bolt.DB, email string) (UserInfo, bool, error) {
	var rval UserInfo
	found := false
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		rval, found, err = extractInfo(tx, email)
		return err
	})
	return rval, found, err
}

/*
GetAllUsers gets all the user info objects from the DB
*/
func GetAllUsers(db *bolt.DB) ([]UserInfo, error) {
	var rval []UserInfo
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte("auth.userinfo"))
		if b != nil {
			c := b.Cursor()
			key, val := c.First()
			for ; key != nil; key, val = c.Next() {
				if val != nil {
					var info UserInfo
					err = json.Unmarshal(val, &info)
					if err == nil {
						rval = append(rval, info)
					}
				}
			}
		}
		return err
	})
	return rval, err
}

/*
UsersWithRole returns a list of users with provided role
*/
func UsersWithRole(db *bolt.DB, role string) ([]string, error) {
	var rval []string
	var info UserInfo
	err := db.View(func(tx *bolt.Tx) error {
		var err error
		b := tx.Bucket([]byte("auth.userinfo"))
		if b != nil {
			c := b.Cursor()
			key, val := c.First()
			for ; key != nil; key, val = c.Next() {
				if val != nil {
					err = json.Unmarshal(val, &info)
					if err == nil && UserHasRole(info, role) {
						rval = append(rval, info.Email)
					}
				}
			}
		}
		return err
	})
	return rval, err
}

/*
AddRole adds the role to the user associated with email
if create is true, user is created if not found
returns true if user info was found
*/
func AddRole(db *bolt.DB, email, role string, create bool) (UserInfo, bool, error) {
	var rval UserInfo
	found := false
	err := db.Update(func(tx *bolt.Tx) error {
		var err error
		rval, found, err = extractInfo(tx, email)
		if found {
			rval.Roles = setAdd(rval.Roles, role)
			err = saveInfo(tx, rval)
		} else if create {
			rval = UserInfo{email, []string{role}}
			err = saveInfo(tx, rval)
		}

		return err
	})
	return rval, found, err
}

/*
RemoveRole removes the role from the user associated with email
returns true if the user info was found
*/
func RemoveRole(db *bolt.DB, email, role string) (UserInfo, bool, error) {
	var rval UserInfo
	found := false
	err := db.Update(func(tx *bolt.Tx) error {
		var err error
		rval, found, err = extractInfo(tx, email)
		if found {
			rval.Roles = setRemove(rval.Roles, role)
			err = saveInfo(tx, rval)
		}

		return err
	})
	return rval, found, err
}

/*
toSet converts a slice to a set
*/
func toSet(arr []string) map[string]bool {
	rval := make(map[string]bool)
	for _, s := range arr {
		rval[s] = true
	}
	return rval
}

/*
toSlice converts a set to a slice
*/
func toSlice(set map[string]bool) []string {
	rval := make([]string, len(set))
	i := 0
	for key := range set {
		rval[i] = key
		i += 1
	}
	return rval
}

/*
setAdd treats the slice as a set and adds an item
new slice is returned, order is not preserved
*/
func setAdd(arr []string, item string) []string {
	setMap := toSet(arr)
	setMap[item] = true
	return toSlice(setMap)
}

/*
setRemove treats the slice as a set and removes an item
new slice is returned, order is not preserved
*/
func setRemove(arr []string, item string) []string {
	setMap := toSet(arr)
	delete(setMap, item)
	return toSlice(setMap)
}

/*
exractInfo extracts a user's info object from the transaction
returns true if info object was found in DB
*/
func extractInfo(tx *bolt.Tx, email string) (UserInfo, bool, error) {
	var rval UserInfo
	found := false
	var err error
	b := tx.Bucket([]byte("auth.userinfo"))
	if b != nil {
		encoded := b.Get([]byte(email))
		if encoded != nil {
			found = true
			err = json.Unmarshal(encoded, &rval)
		}
	}
	return rval, found, err
}

/*
saveInfo saves the info object into the DB
*/
func saveInfo(tx *bolt.Tx, info UserInfo) error {
	var err error
	b := tx.Bucket([]byte("auth.userinfo"))
	if b != nil {
		var encoded []byte
		encoded, err = json.Marshal(info)
		b.Put([]byte(info.Email), encoded)
	}
	return err
}

/*
HasRole returns true if the user associated with email has the provided role
*/
func HasRole(db *bolt.DB, email, role string) bool {
	rval := false
	info, found, err := GetUserInfo(db, email)
	if found {
		rval = UserHasRole(info, role)
	}
	if err != nil {
		log.Printf("Problem checking roles %v\n", err)
	}
	return rval
}

/*
UserHasRole returns true if userinfo contains given role
*/
func UserHasRole(info UserInfo, role string) bool {
	rval := false
	for _, userRole := range info.Roles {
		if role == userRole {
			rval = true
			break
		}
	}
	return rval
}

/*
RegisterAuth sets up authentication handlers in the mux router.
If useOAuth is false, a test auth handler is used instead.
*/
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

/*
handleTestAuth is a dummy auth handler that only authenticates the test user
*/
func handleTestAuth(w http.ResponseWriter, r *http.Request) *AppError {
	returnPath := r.FormValue("return")
	if returnPath == "" {
		returnPath = "/"
	}
	session, _ := store.Get(r, SessionName)
	session.Values[EmailKey] = "test@example.com"
	session.Values[NameKey] = "Test User"

	session.Save(r, w)
	http.Redirect(w, r, returnPath, http.StatusFound)
	return nil
}

/*
getLoginInfo gets the session login info from the cookies in the request
*/
func getLoginInfo(r *http.Request) *LoginInfo {
	session, _ := store.Get(r, SessionName)
	email := getSessionValue(session, EmailKey)
	name := getSessionValue(session, NameKey)
	return &LoginInfo{email, name}
}

/*
getSessionValue gets a cookie value from the session
*/
func getSessionValue(session *sessions.Session, key string) string {
	rval, _ := session.Values[key].(string)
	return rval
}

/*
handleLogout is a handler that ends the users session
*/
func handleLogout(w http.ResponseWriter, r *http.Request) *AppError {
	returnPath := r.FormValue("return")
	if returnPath == "" {
		returnPath = "/"
	}
	session, _ := store.Get(r, SessionName)
	session.Values[EmailKey] = ""
	session.Values[NameKey] = ""
	/* this is the suggested way to delete a session,
	   but firefox seems to ignore a cookie that is already expired */
	//session.Options = &sessions.Options{MaxAge: -1}
	session.Save(r, w)
	http.Redirect(w, r, returnPath, http.StatusFound)
	return nil
}

/*
loginHandler refers the user to the specified OAuth provider for authentication
*/
func loginHandler(providerName string) Wrapper {
	provider, err := gomniauth.Provider(providerName)
	if err != nil {
		panic(err)
	}
	f := func(w http.ResponseWriter, r *http.Request) *AppError {
		returnPath := r.FormValue("return")

		state := gomniauth.NewState("after", returnPath)
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

/*
callBackHandler creates a new handler for the OAuth provider
*/
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
		target := "/"
		stateParam := r.FormValue("state")
		state, err := gomniauth.StateFromParam(stateParam)
		if err == nil {
			after := state.Get("after").Str("/")
			if after != "" {
				target = after
			}
		}
		http.Redirect(w, r, target, http.StatusFound)
		return nil
	}
	return Wrapper{HandlerFunc(f)}
}
