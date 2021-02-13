package unclog

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/bobg/aesite"
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

func (s *Server) handleData(w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()

	var data homedata

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if aesite.IsNoSession(err) {
		sess, err = aesite.NewSession(ctx, s.dsClient, nil)
		if err != nil {
			return errors.Wrap(err, "creating new session")
		}
		sess.SetCookie(w)
	} else if err != nil {
		return errors.Wrap(err, "getting session")
	} else {
		var u user
		err = sess.GetUser(ctx, s.dsClient, &u)
		if errors.Is(err, aesite.ErrAnonymous) {
			// ok
		} else if err != nil {
			return errors.Wrap(err, "getting session user")
		} else {
			data.Email = u.Email
			if u.Token != "" {
				client, err := s.oauthClient(ctx, &u)
				if err != nil {
					return errors.Wrapf(err, "creating oauth client for %s", u.Email)
				}
				gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
				if err != nil {
					return errors.Wrapf(err, "creating gmail client for %s", u.Email)
				}
				_, err = gmailSvc.Users.GetProfile("me").Do()
				if err != nil {
					log.Printf("Getting profile for %s: %s", u.Email, err)
					u.Token = ""
					_, err = s.dsClient.Put(ctx, u.Key(), &u)
					if err != nil {
						return errors.Wrapf(err, "updating user %s after token expiry", u.Email)
					}
					data.Expired = true
				} else {
					// xxx check prof.EmailAddress == u.Email?
					data.Enabled = u.WatchExpiry.After(time.Now())
				}
			}
		}
	}

	csrf, err := sess.CSRFToken()
	if err != nil {
		return errors.Wrap(err, "creating CSRF token")
	}
	data.Csrf = csrf

	err = json.NewEncoder(w).Encode(data)
	return errors.Wrap(err, "rendering JSON response")
}
