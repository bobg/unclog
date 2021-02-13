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

	// InboxOnly tells whether only messages in the Inbox are checked for labeling/unlabeling.
	// If false, all new messages are checked.
	InboxOnly bool

	// Token is the user's OAuth token, if any.
	Token string

	// ContactsLabelID is the user's Gmail id for unstarred contacts.
	ContactsLabelID string

	// StarredLabelID is the user's Gmail id for starred contacts.
	StarredLabelID string

	// NextUpdate is set when a new update task is queued, to prevent a second from being queued too soon.
	// See Server.queueUpdate.
	NextUpdate time.Time

	// LastUpdate is the time at which an update task last ran.
	LastUpdate time.Time

	// LastThreadTime is the latest timestamp of a message contemplated in an update task.
	LastThreadTime time.Time

	// WatchExpiry is when the current gmail pubsub subscription expires, if any.
	WatchExpiry time.Time
}

// GetUser implements aesite.UserWrapper.
func (u *user) GetUser() *aesite.User {
	return &u.User
}

// GetUser implements aesite.UserWrapper.
func (u *user) SetUser(au *aesite.User) {
	u.User = *au
}

// GET/POST /s/enable
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

	var u user
	err = sess.GetUser(ctx, s.dsClient, &u)
	if err != nil {
		return errors.Wrap(err, "getting user record")
	}

	// now := time.Now()
	// if u.WatchExpiry.After(now) {
	// 	// xxx already enabled
	// }

	err = s.watch(ctx, &u)
	if err != nil {
		return errors.Wrap(err, "renewing gmail push-notice subscription")
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)

	return nil
}

// GET/POST /s/disable
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
		return nil
	}

	err = s.stop(ctx, &u)
	if err != nil {
		return errors.Wrap(err, "stopping gmail push-notice subscription")
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)

	return nil
}
