package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"
	"strconv"
	"errors"
	"strings"

	"github.com/gorilla/sessions"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"github.com/samanaphone/agents/pkg/ami"
)

// GoogleUser holds the user info returned from Google
type GoogleUser struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
}

var (
	oauthConfig *oauth2.Config
	amiConfig   *ami.Config
	store       *sessions.CookieStore
	templates   *template.Template
	queueName   map[string]string
	qFileName   string
)

func init() {
	// Session store — use a strong random key in production
	sessionKey := getEnv("SESSION_KEY", "super-secret-session-key-change-in-prod")
	store = sessions.NewCookieStore([]byte(sessionKey))
	store.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 1, // 1 days
		HttpOnly: true,
		Secure:   true, // Set to true in production (HTTPS)
	}

	// OAuth2 config
	oauthConfig = &oauth2.Config{
		ClientID:     getEnv("GOOGLE_CLIENT_ID", "YOUR_GOOGLE_CLIENT_ID"),
		ClientSecret: getEnv("GOOGLE_CLIENT_SECRET", "YOUR_GOOGLE_CLIENT_SECRET"),
		RedirectURL:  getEnv("REDIRECT_URL", "http://localhost:8080/auth/callback"),
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/userinfo.profile",
		},
		Endpoint: google.Endpoint,
	}

	amiPort, _ := strconv.Atoi(getEnv("AMI_PORT", "8088"))
	amiTimeoutSec, err := strconv.Atoi(getEnv("AMI_TIMEOUT_SECONDS", "60"))
	if err != nil {
		amiTimeoutSec = 60
	}
	amiTls, err := strconv.ParseBool(getEnv("AMI_TLS", "false"))
	if err != nil {
		amiTls = false
	}

	amiConfig = &ami.Config{
		Host: getEnv("AMI_HOST", "0.0.0.0"),
		Port: amiPort,
		Username: getEnv("AMI_USERNAME", "none"),
		Secret: getEnv("AMI_PASSWORD", "none"),
		Timeout: time.Duration(amiTimeoutSec) * time.Second,
		TLS: amiTls,
	}

	// Parse templates
	templates = template.Must(template.ParseGlob("templates/*.html"))

	queueName = map[string]string{}

}

func main() {

	var err error

	queueName, err = dbGetTree("Samana", "queue/name")
	if err != nil {
		log.Printf("Error: %v", err)
	}
	log.Printf("Queues: %+v", queueName)


	mux := http.NewServeMux()

	// Public routes
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/login", handleLogin)
	mux.HandleFunc("/auth/callback", handleCallback)
	mux.HandleFunc("/logout", handleLogout)
	mux.HandleFunc("/favicon.ico", handleFavIcon)

	// Protected routes (wrapped with auth middleware)
	mux.Handle("/dashboard", requireAuth(http.HandlerFunc(handleDashboard)))
	mux.Handle("/profile", requireAuth(http.HandlerFunc(handleProfile)))
	mux.Handle("/queuestatus", requireAuth(http.HandlerFunc(handleQueueStatus)))
	mux.Handle("/joinqueue/{queue}", requireAuth(http.HandlerFunc(handleJoinQueue)))
	mux.Handle("/leavequeue/{queue}", requireAuth(http.HandlerFunc(handleLeaveQueue)))
	mux.Handle("/pausemember/{queue}", requireAuth(http.HandlerFunc(handlePauseMember)))
	mux.Handle("/resumemember/{queue}", requireAuth(http.HandlerFunc(handleResumeMember)))
	mux.Handle("/dbget/{extension}", requireAuth(http.HandlerFunc(handleDBGet)))
	mux.Handle("/updatenumber/{newnumber}", requireAuth(http.HandlerFunc(handleUpdateNumber)))
	mux.Handle("/users", requireAuth(http.HandlerFunc(handleUsers)))
	mux.Handle("/adduser/{email}/{extension}/{roles}", requireAuth(http.HandlerFunc(handleAddUser)))
	mux.Handle("/deluser/{email}", requireAuth(http.HandlerFunc(handleDeleteUser)))

	// Static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	port := getEnv("PORT", "8088")
	log.Printf("🚀 Server running at http://0.0.0.0:%s", port)
	log.Fatal(http.ListenAndServe("0.0.0.0:"+port, mux))
}

// ── Handlers ──────────────────────────────────────────────────────────────────

func handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	user := getSessionUser(r)
	renderTemplate(w, "home.html", map[string]any{
		"User": user,
	})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	// Already logged in? Send to dashboard
	if getSessionUser(r) != nil {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}

	state := generateState()
	session, _ := store.Get(r, "oauth-state")
	session.Values["state"] = state
	session.Values["state_expiry"] = time.Now().Add(10 * time.Minute).Unix()
	session.Save(r, w)

	url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOnline)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleCallback(w http.ResponseWriter, r *http.Request) {
	// Validate state
	session, _ := store.Get(r, "oauth-state")
	savedState, ok := session.Values["state"].(string)
	expiry, _ := session.Values["state_expiry"].(int64)

	if !ok || savedState != r.URL.Query().Get("state") {
		http.Error(w, "Invalid OAuth state", http.StatusBadRequest)
		return
	}
	if time.Now().Unix() > expiry {
		http.Error(w, "OAuth state expired, please try again", http.StatusBadRequest)
		return
	}

	// Clear state
	delete(session.Values, "state")
	delete(session.Values, "state_expiry")
	session.Save(r, w)

	// Exchange code for token
	code := r.URL.Query().Get("code")
	token, err := oauthConfig.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("Token exchange error: %v", err)
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}
	log.Printf("Token scopes: %v", token.Extra("scope"))


	// Fetch user info
	googleUser, err := fetchGoogleUser(token)
	if err != nil {
		log.Printf("Failed to fetch user info: %v", err)
		http.Error(w, "Failed to fetch user info", http.StatusInternalServerError)
		return
	}

	extension, roles, err := fetchExtension(googleUser.Email)
	if err != nil {
	    log.Printf("Could not fetch custom attributes: %v", err)
	}

	// Save user to session
	userSession, _ := store.Get(r, "user-session")
	userSession.Values["user_id"] = googleUser.ID
	userSession.Values["user_email"] = googleUser.Email
	userSession.Values["user_name"] = googleUser.Name
	userSession.Values["user_picture"] = googleUser.Picture
	userSession.Values["logged_in"] = true
	userSession.Values["extension"] = extension
	userSession.Values["roles"] = roles
	userSession.Save(r, w)

	log.Printf("✅ User logged in: %s (%s)", googleUser.Name, googleUser.Email)
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

func handleLogout(w http.ResponseWriter, r *http.Request) {
	session, _ := store.Get(r, "user-session")
	session.Options.MaxAge = -1 // Delete session
	session.Save(r, w)
	http.Redirect(w, r, "/", http.StatusFound)
}

func handleDashboard(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)
	queues, _ := myQueues(user.Interface)
	followme, _ := getFollowMeNumber(user.Extension)

	if ! user.HasRole("agent") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	renderTemplate(w, "dashboard.html", map[string]any{
		"User": user,
		"Queues": queues,
		"FollowMe": followme,
	})
}

func handleFavIcon(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/static/cropped-samana-icon-32x32.png", 301)
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)
	renderTemplate(w, "profile.html", map[string]any{
		"User": user,
	})
}

func queueStatus() ([]*ami.QueueParamsEvent, error) {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		return nil, errors.New("Unable to login: " + err.Error())
	}

	qdata, err := client.QueueStatus(ctx)
	if err != nil {
		return nil, errors.New("Error getting queue status: " + err.Error())
	}
	queues := ami.ParseQueueStatus(qdata)

	if err := client.Logoff(ctx); err != nil {
		return nil, errors.New("Unable to logoff: " + err.Error())
	}
	return queues, nil
}

func handleQueueStatus(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)

	if ! user.HasRole("admin") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	qs, err := queueStatus()
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	queueStats := map[string]any{}
	for _, queue := range qs {
		if queue.Queue == "default" {
			continue
		}
		qn, _ := queueName[queue.Queue]
		if qn == "" {
			qn = queue.Queue
		}
		am := []any{}
		im := []any{}
		for _, member := range queue.Members {
			m := map[string]any{
				"Name": member.Name,
				"CallsTaken": member.CallsTaken,
				"LastCall": time.Now().Sub(time.Unix(member.LastCall, 0)).Truncate(time.Second).String(),
				"Status": member.Status.String(),
				"Paused": member.Paused,
			}
			if member.Paused || member.Status.String() == "Unavailable" {
				im = append(im, m)
			} else {
				am = append(am, m)
			}
		}

		queueStats[qn] = map[string]any{
			"Calls": queue.Calls,
			"Completed": queue.Completed,
			"Abandoned": queue.Abandoned,
			"Holdtime": queue.Holdtime,
			"TalkTime": queue.TalkTime,
			"ServiceLevel": queue.ServiceLevel,
			"ActiveMembers": am,
			"InactiveMembers": im,
			"CountActiveMembers": len(am),
		}
	}

	data := map[string]any{
		"User": user,
		"Queues": queueStats,
	}
	renderTemplate(w, "queuestatus.html", data)
}

func joinQueue(queue, iface, name, stateInterface string) (map[string]string, error) {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		return nil, errors.New("Unable to login: " + err.Error())
	}

	res, err := client.QueueAdd(ctx, queue, iface, name, stateInterface)
	if err != nil {
		return nil, errors.New("Error adding member to queue: " + err.Error())
	}

	if err := client.Logoff(ctx); err != nil {
		return nil, errors.New("Unable to logoff: " + err.Error())
	}

	if ! res.IsSuccess() {
		msg, _ := res.Fields["message"]
		return nil, errors.New("Error Adding member: " + msg)
	}

	return res.Fields, nil
}

func handleJoinQueue(w http.ResponseWriter, r *http.Request) {
	queue := r.PathValue("queue")
	user := getSessionUser(r)

	if ! user.HasRole("agent") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	_, err := joinQueue(queue, user.Interface, user.Name, user.StateInterface)
	if err != nil {
		log.Printf("%v", err)
	}
	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}

func leaveQueue(queue, iface string) (map[string]string, error) {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		return nil, errors.New("Unable to login: " + err.Error())
	}

	res, err := client.QueueRemove(ctx, queue, iface)
	if err != nil {
		return nil, errors.New("Error removing member from queue: " + err.Error())
	}

	if err := client.Logoff(ctx); err != nil {
		return nil, errors.New("Unable to logoff: " + err.Error())
	}

	if ! res.IsSuccess() {
		msg, _ := res.Fields["message"]
		return nil, errors.New("Error removing member: " + msg)
	}

	return res.Fields, nil
}

func handleLeaveQueue(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)
	queue := r.PathValue("queue")

	if ! user.HasRole("agent") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	_, err := leaveQueue(queue, user.Interface)
	if err != nil {
		log.Printf("%v", err)
	}
	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}

type memberStatus struct {
	Queue string
	Name string
	Joined bool
	Paused bool
	Dynamic bool
}

func myQueues(iface string) ([]memberStatus, error) {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		return nil, errors.New("Unable to login: " + err.Error())
	}

	qdata, err := client.QueueStatus(ctx)
	if err != nil {
		return nil, errors.New("Error getting queue status: " + err.Error())
	}

	queues := ami.ParseQueueStatus(qdata)

	if err := client.Logoff(ctx); err != nil {
		return nil, errors.New("Unable to logoff: " + err.Error())
	}

	qms := []memberStatus{}

	for _, queue := range queues {
		if queue.Queue == "default" {
			continue
		}
		qn, _ := queueName[queue.Queue]
		if qn == "" {
			qn = "—"
		}
		ms := memberStatus{
			Queue: queue.Queue,
			Name: qn,
		}
		for _, member := range queue.Members {
			if member.Location == iface {
				ms.Joined = true
				ms.Paused = member.Paused
				ms.Dynamic = member.Membership == "dynamic"
				break
			}
		}
		qms = append(qms, ms)
	}

	return qms, nil
}

func handleMyQueues(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)

	if ! user.HasRole("agent") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	qm, err := myQueues(user.Interface)
	if err != nil {
		log.Printf("%v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return		
	}

	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(qm); err != nil {
		log.Printf("Unable to encode data: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
}

func pauseMember(queue, iface string, pause bool) error {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		return errors.New("Unable to login: " + err.Error())
	}

	res, err := client.QueuePause(ctx, queue, iface, pause)
	if err != nil {
		return errors.New("Error pausing member: " + err.Error())
	}

	if err := client.Logoff(ctx); err != nil {
		return errors.New("Unable to logoff: " + err.Error())
	}

	if ! res.IsSuccess() {
		msg, _ := res.Fields["message"]
		return errors.New("Error removing member: " + msg)
	}

	return nil	
}

func handlePauseMember(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)
	queue := r.PathValue("queue")
	if queue == "all" {
		queue = ""
	}

	if ! user.HasRole("agent") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	err := pauseMember(queue, user.Interface, true)
	if err != nil {
		log.Printf("%v", err)
	}
	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}

func handleResumeMember(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)

	if ! user.HasRole("agent") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	queue := r.PathValue("queue")
	if queue == "all" {
		queue = ""
	}

	err := pauseMember(queue, user.Interface, false)
	if err != nil {
		log.Printf("%v", err)
	}
	http.Redirect(w, r, "/dashboard", http.StatusTemporaryRedirect)
}

func getFollowMeNumber(extension string) (string, error) {
	num, err := dbGet("AMPUSER", extension + "/followme/grplist")
	if err != nil {
		fmt.Printf("Error: %v", err)
		return "", err
	}

	if num[0] == '9' {
		num = num[1:]
	}
	if num[len(num)-1] == '#' {
		num = num[:len(num)-1]
	}
	return num, nil
}

func dbGet(family, key string) (string, error) {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		return "", errors.New("Unable to login: " + err.Error())
	}

	res, err := client.DBGet(ctx, family, key)
	if err != nil {
		return "", errors.New("Error adding member to queue: " + err.Error())
	}

	if err := client.Logoff(ctx); err != nil {
		return "", errors.New("Unable to logoff: " + err.Error())
	}

	if ! res.IsSuccess() {
		msg, _ := res.Fields["message"]
		return "", errors.New("Error Adding member: " + msg)
	}

	for _, event := range res.Events {
		eventType, _ := event["event"]
		if eventType != "DBGetResponse" {
			continue
		}
		value, _ := event["val"]
		return value, nil
	}

	return "", errors.New("Unknown error")
}

func dbGetTree(family, key string) (map[string]string, error) {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	values := map[string]string{}
	if err := client.Login(ctx); err != nil {
		return values, errors.New("Unable to login: " + err.Error())
	}

	res, err := client.DBGetTree(ctx, family, key)
	if err != nil {
		return values, errors.New("Error getting db tree: " + err.Error())
	}

	if err := client.Logoff(ctx); err != nil {
		return values, errors.New("Unable to logoff: " + err.Error())
	}

	if ! res.IsSuccess() {
		msg, _ := res.Fields["message"]
		return values, errors.New("Error getting db tree: " + msg)
	}

	for _, event := range res.Events {
		eventType, _ := event["event"]
		if eventType != "DBGetTreeResponse" {
			continue
		}
		k := event["key"]
		prefix := "/" + family + "/" + key + "/"
		if len(k) > len(prefix) {
			k = k[len(prefix):]
		}
		values[k] = event["val"]
	}

	return values, nil
}

func dbDelTree(family, key string) error {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		return errors.New("Unable to login: " + err.Error())
	}

	res, err := client.DBDelTree(ctx, family, key)
	if err != nil {
		return errors.New("Error removing db tree: " + err.Error())
	}

	if err := client.Logoff(ctx); err != nil {
		return errors.New("Unable to logoff: " + err.Error())
	}

	if ! res.IsSuccess() {
		msg, _ := res.Fields["message"]
		return errors.New("Error removing db tree: " + msg)
	}

	return nil
}

func handleDBGet(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)

	if ! user.HasRole("agent") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	res, err := dbGet("AMPUSER", "5101/followme/grplist")
	if err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}

	enc := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := enc.Encode(res); err != nil {
		log.Printf("Error: %v", err)
		http.Error(w, "", http.StatusInternalServerError)
		return
	}
}

func putFollowMeNumber(extension, newnumber string) error {
	err := dbPut("AMPUSER", extension + "/followme/grplist", "9" + newnumber + "#")
	if err != nil {
		fmt.Printf("Error: %v", err)
		return err
	}
	return err
}

func dbPut(family, key, value string) (error) {
	client := ami.NewClient(amiConfig)
	ctx := context.Background()

	if err := client.Login(ctx); err != nil {
		return errors.New("Unable to login: " + err.Error())
	}

	res, err := client.DBPut(ctx, family, key, value)
	if err != nil {
		return errors.New("Error adding member to queue: " + err.Error())
	}

	if err := client.Logoff(ctx); err != nil {
		return errors.New("Unable to logoff: " + err.Error())
	}

	if ! res.IsSuccess() {
		msg, _ := res.Fields["message"]
		return errors.New("Error Adding member: " + msg)
	}

	return nil
}

func handleUpdateNumber(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)

	if ! user.HasRole("agent") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}


	newnumber := r.PathValue("newnumber")
	_, err := strconv.Atoi(newnumber)
	if err != nil || newnumber == ""  || len(newnumber) > 30 {
		log.Printf("Error: invalid new number '%s'", newnumber)
	} else {
		putFollowMeNumber(user.Extension, newnumber)
	}
	http.Redirect(w, r, "/dashboard", http.StatusFound)
}

type user struct {
	Extension string
	Roles string
}

func listUsers() (map[string]*user, error) {
	users, err := dbGetTree("Samana", "users")
	out := map[string]*user{}

	if err != nil {
		return out, err
	}

	for k, v := range users {
		username, attr, _ := strings.Cut(k, "/")
		u, found := out[username]
		if ! found {
			u = &user{}
			out[username] = u
		}
		switch attr {
		case "role":
			u.Roles = v
		case "extension":
			u.Extension = v
		}
	}
	return out, nil
}

func handleUsers(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)

	if ! user.HasRole("admin") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}
	users, err := listUsers()
	if err != nil {
		log.Printf("Error: '%v'", err)
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	data := map[string]any{
		"User": user,
		"Users": users,
	}
	renderTemplate(w, "users.html", data)
}

func handleAddUser(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)

	if ! user.HasRole("admin") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	email := r.PathValue("email")
	extension := r.PathValue("extension")
	roles := r.PathValue("roles")

	err := dbPut("Samana", "users/" + email + "/extension", extension)
	if err != nil {
		log.Printf("Error: %+v", err)
	}
	err = dbPut("Samana", "users/" + email + "/role", roles)
	if err != nil {
		log.Printf("Error: %+v", err)
	}

	http.Redirect(w, r, "/users", http.StatusFound)
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	user := getSessionUser(r)

	if ! user.HasRole("admin") {
		http.Redirect(w, r, "/profile", http.StatusFound)
		return
	}

	email := r.PathValue("email")


	err := dbDelTree("Samana", "users/" + email)
	if err != nil {
		log.Printf("Error: %+v", err)
	}

	http.Redirect(w, r, "/users", http.StatusFound)
}
// ── Middleware ─────────────────────────────────────────────────────────────────

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if getSessionUser(r) == nil {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

type SessionUser struct {
	ID             string
	Email          string
	Name           string
	Picture        string
	Extension      string
	Interface      string
	StateInterface string
	Roles          []string
}

func (self *SessionUser) HasRole(rolequery string) bool {
	for _, role := range self.Roles {
		if role == rolequery {
			return true
		}
	}
	return false
}

func getSessionUser(r *http.Request) *SessionUser {
	session, err := store.Get(r, "user-session")
	if err != nil {
		return nil
	}
	loggedIn, _ := session.Values["logged_in"].(bool)
	if !loggedIn {
		return nil
	}
	return &SessionUser{
		ID:             session.Values["user_id"].(string),
		Email:          session.Values["user_email"].(string),
		Name:           session.Values["user_name"].(string),
		Picture:        session.Values["user_picture"].(string),
		Extension:      session.Values["extension"].(string),
		Interface:      "Local/" + session.Values["extension"].(string) + "@from-queue/n",
		StateInterface: "hint:" + session.Values["extension"].(string) + "@ext-local",
		Roles:          session.Values["roles"].([]string),
	}
}

func fetchGoogleUser(token *oauth2.Token) (*GoogleUser, error) {
	client := oauthConfig.Client(context.Background(), token)
	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var user GoogleUser
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func generateState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := templates.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Template error (%s): %v", name, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// Suppress unused import warning when building without fmt usage
var _ = fmt.Sprintf

func fetchExtension(userEmail string) (string, []string, error) {
	userdata, err := dbGetTree("Samana", "users/" + userEmail)
	if err != nil {
		return "", []string{}, err
	}

	log.Printf("ExtensionData: %+v", userdata)

	extension, _ := userdata["extension"]
	rolestr, _ := userdata["role"]
    roles := strings.Split(rolestr, ",")

    return extension, roles, nil
}
