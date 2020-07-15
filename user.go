package unclog

import (
	"net/http"
	"time"

	"github.com/bobg/aesite"
	"github.com/pkg/errors"
)

const pubsubTopic = "projects/unclog/topics/gmail"

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

func (s *Server) handleEnable(w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if err != nil {
		return errors.Wrap(err, "getting session")
	}

	csrf := req.FormValue("csrf")
	err = sess.CSRFCheck(csrf)
	if err != nil {
		return errors.Wrap(err, "checking CSRF token")
	}

	now := time.Now()

	var u user
	err = sess.GetUser(ctx, s.dsClient, &u)
	if err != nil {
		return errors.Wrap(err, "getting user record")
	}

	if u.WatchExpiry.After(now) {
		// xxx already enabled
	}

	err = s.watch(ctx, &u)
	if err != nil {
		return errors.Wrap(err, "renewing gmail push-notice subscription")
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)

	return nil
}

func (s *Server) handleDisable(w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if err != nil {
		return errors.Wrap(err, "getting session")
	}

	csrf := req.FormValue("csrf")
	err = sess.CSRFCheck(csrf)
	if err != nil {
		return errors.Wrap(err, "checking CSRF token")
	}

	now := time.Now()

	var u user
	err = sess.GetUser(ctx, s.dsClient, &u)
	if err != nil {
		return errors.Wrap(err, "getting user")
	}

	if u.WatchExpiry.Before(now) {
		// xxx already disabled
	}

	err = s.stop(ctx, &u)
	if err != nil {
		return errors.Wrap(err, "stopping gmail push-notice subscription")
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)

	return nil
}
