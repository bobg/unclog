package unclog

import (
	"context"

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

func (s *Server) oauthConf(ctx context.Context) (*oauth2.Config, error) {
	oauthConfJSON, err := aesite.GetSetting(ctx, s.dsClient, "oauthConf")
	if err != nil {
		return nil, errors.Wrap(err, "getting oauth config")
	}
	return google.ConfigFromJSON(oauthConfJSON, scopes...)
}
