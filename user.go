package unclog

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/bobg/aesite"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type user struct {
	aesite.User

	// ContactsLabelID is the Gmail label ID of this user's "Contacts" label.
	ContactsLabelID string

	// StarredLabelID is the Gmail label ID of this user's "Contacts/Starred" label.
	StarredLabelID string

	// LeaseExpiry is the time when this user will be available to handle a push request.
	LeaseExpiry time.Time

	InboxOnly      bool
	LastThreadTime time.Time
	WatchExpiry    time.Time
	Token          string
}

func (u *user) GetUser() *aesite.User {
	return &u.User
}

func (u *user) SetUser(au *aesite.User) {
	u.User = *au
}

func (s *Server) handleEnable(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting session: %s", err), http.StatusInternalServerError)
		return
	}

	csrf := req.FormValue("csrf")
	err = sess.CSRFCheck(csrf)
	if err != nil {
		http.Error(w, fmt.Sprintf("checking CSRF token: %s", err), http.StatusInternalServerError)
		return
	}

	now := time.Now()

	var u user
	err = sess.GetUser(ctx, s.dsClient, &u)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting user: %s", err), http.StatusInternalServerError)
		return
	}

	if u.WatchExpiry.After(now) {
		// xxx already enabled
	}

	if u.Token == "" {
		http.Error(w, "oauth token required", http.StatusUnauthorized)
		// xxx cancel session?
		return
	}

	var token oauth2.Token
	err = json.Unmarshal([]byte(u.Token), &token)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not decode oauth token: %s", err), http.StatusInternalServerError)
		return
	}

	oauthConf, err := s.oauthConf(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("configuring oauth: %s", err), http.StatusInternalServerError)
		return
	}
	oauthClient := oauthConf.Client(ctx, &token)

	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		http.Error(w, fmt.Sprintf("allocating gmail service: %s", err), http.StatusInternalServerError)
		return
	}

	watchReq := &gmail.WatchRequest{
		TopicName: pubsubTopic,
	}
	watchResp, err := gmailSvc.Users.Watch("me", watchReq).Do()
	if err != nil {
		http.Error(w, fmt.Sprintf("subscribing to gmail push notices: %s", err), http.StatusInternalServerError)
		return
	}
	u.WatchExpiry = timeFromMillis(watchResp.Expiration)

	_, err = s.dsClient.Put(ctx, u.Key(), u)
	if err != nil {
		http.Error(w, fmt.Sprintf("storing updated user record: %s", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)
}

func (s *Server) handleDisable(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting session: %s", err), http.StatusInternalServerError)
		return
	}

	csrf := req.FormValue("csrf")
	err = sess.CSRFCheck(csrf)
	if err != nil {
		http.Error(w, fmt.Sprintf("checking CSRF token: %s", err), http.StatusInternalServerError)
		return
	}

	now := time.Now()

	var u user
	err = sess.GetUser(ctx, s.dsClient, &u)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting user: %s", err), http.StatusInternalServerError)
		return
	}

	if u.WatchExpiry.Before(now) {
		// xxx already disabled
	}

	if u.Token == "" {
		http.Error(w, "oauth token required", http.StatusUnauthorized)
		// xxx cancel session?
		return
	}

	var token oauth2.Token
	err = json.Unmarshal([]byte(u.Token), &token)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not decode oauth token: %s", err), http.StatusInternalServerError)
		return
	}

	oauthConf, err := s.oauthConf(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("configuring oauth: %s", err), http.StatusInternalServerError)
		return
	}
	oauthClient := oauthConf.Client(ctx, &token)

	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		http.Error(w, fmt.Sprintf("allocating gmail service: %s", err), http.StatusInternalServerError)
		return
	}

	err = gmailSvc.Users.Stop("me").Do()
	if err != nil {
		http.Error(w, fmt.Sprintf("unsubscribing from Gmail push notifications: %s", err), http.StatusInternalServerError)
		return
	}

	u.WatchExpiry = time.Time{}

	_, err = s.dsClient.Put(ctx, u.Key(), u)
	if err != nil {
		http.Error(w, fmt.Sprintf("storing updated user record: %s", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)
}
