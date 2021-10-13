package unclog

import (
	"context"
	"log"
	"time"

	"github.com/bobg/aesite"
	"github.com/bobg/mid"
	"github.com/pkg/errors"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type homedata struct {
	// Csrf is always present.
	Csrf string `json:"csrf"`

	// Email is sometimes present.
	Email string `json:"email"`

	// The following are only present if Email is.
	Enabled bool `json:"enabled"`
	Expired bool `json:"expired"`
}

// GET /s/data
func (s *Server) handleData(ctx context.Context) (*homedata, error) {
	sess, err := aesite.GetSession(ctx, s.dsClient, mid.Request(ctx))
	if aesite.IsNoSession(err) {
		return s.newSession(ctx)
	}
	if err != nil {
		return nil, errors.Wrap(err, "getting session")
	}
	return s.existingSession(ctx, sess)
}

const (
	dayDur     = 24 * time.Hour
	sessionDur = 365 * dayDur
)

func (s *Server) newSession(ctx context.Context) (*homedata, error) {
	sess, err := aesite.NewSessionWithDuration(ctx, s.dsClient, nil, sessionDur)
	if err != nil {
		return nil, errors.Wrap(err, "creating new session")
	}
	sess.SetCookie(mid.ResponseWriter(ctx))
	return respondWithCSRFAndData(ctx, sess, homedata{})
}

func (s *Server) existingSession(ctx context.Context, sess *aesite.Session) (*homedata, error) {
	var (
		u    user
		data homedata
	)

	err := sess.GetUser(ctx, s.dsClient, &u)
	if errors.Is(err, aesite.ErrAnonymous) {
		// ok
	} else if err != nil {
		return nil, errors.Wrap(err, "getting session user")
	} else {
		data.Email = u.Email
		if u.Token != "" {
			client, err := s.oauthClient(ctx, &u)
			if err != nil {
				return nil, errors.Wrapf(err, "creating oauth client for %s", u.Email)
			}
			gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
			if err != nil {
				return nil, errors.Wrapf(err, "creating gmail client for %s", u.Email)
			}
			_, err = gmailSvc.Users.GetProfile("me").Do()
			if err != nil {
				log.Printf("Getting profile for %s: %s", u.Email, err)
				u.Token = ""
				_, err = s.dsClient.Put(ctx, u.Key(), &u)
				if err != nil {
					return nil, errors.Wrapf(err, "updating user %s after token expiry", u.Email)
				}
				data.Expired = true
			} else {
				// xxx check prof.EmailAddress == u.Email?
				data.Enabled = u.WatchExpiry.After(time.Now())
			}
		}
	}

	return respondWithCSRFAndData(ctx, sess, data)
}

func respondWithCSRFAndData(ctx context.Context, sess *aesite.Session, data homedata) (*homedata, error) {
	csrf, err := sess.CSRFToken()
	if err != nil {
		return nil, errors.Wrap(err, "creating CSRF token")
	}
	data.Csrf = csrf
	return &data, nil
}
