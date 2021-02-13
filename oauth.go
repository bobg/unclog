package unclog

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bobg/aesite"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/people/v1"
)

var scopes = []string{
	people.ContactsReadonlyScope,
	gmail.GmailLabelsScope,
	gmail.GmailModifyScope,
}

// Caches the oauthConf setting and returns its value.
func (s *Server) getOauthConf(ctx context.Context) (*oauth2.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.oauthConf == nil {
		oauthConfJSON, err := aesite.GetSetting(ctx, s.dsClient, "oauthConf")
		if err != nil {
			return nil, errors.Wrap(err, "getting oauth config")
		}
		conf, err := google.ConfigFromJSON(oauthConfJSON, scopes...)
		if err != nil {
			return nil, errors.Wrap(err, "in ConfigFromJSON")
		}
		s.oauthConf = conf
	}
	return s.oauthConf, nil
}

var errNoToken = errors.New("no token")

// Produces an oauth-authenticated http client for the given user.
func (s *Server) oauthClient(ctx context.Context, u *user) (*http.Client, error) {
	if u.Token == "" {
		return nil, errNoToken
	}

	oauthConf, err := s.getOauthConf(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting oauth config")
	}

	var token oauth2.Token
	err = json.Unmarshal([]byte(u.Token), &token)
	if err != nil {
		return nil, errors.Wrap(err, "JSON-unmarshaling token")
	}

	return oauthConf.Client(ctx, &token), nil
}
