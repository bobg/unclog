package unclog

import (
	"context"
	"encoding/json"
	"net/http"

	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func (s *Server) handleAuth(w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()

	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if err != nil {
		return errors.Wrap(err, "getting session")
	}

	conf, err := s.getOauthConf(ctx)
	if err != nil {
		return errors.Wrap(err, "getting OAuth config")
	}

	url := conf.AuthCodeURL(sess.Key().Encode(), oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, req, url, http.StatusSeeOther)

	return nil
}

// This is where the OAuth flow redirects to.
func (s *Server) handleAuth2(w http.ResponseWriter, req *http.Request) error {
	var (
		ctx   = req.Context()
		code  = req.FormValue("code")
		state = req.FormValue("state")
	)

	sessKey, err := datastore.DecodeKey(state)
	if err != nil {
		return errors.Wrap(err, "decoding session key")
	}
	sess, err := aesite.GetSessionByKey(ctx, s.dsClient, sessKey)
	if err != nil {
		return errors.Wrap(err, "getting session")
	}

	conf, err := s.getOauthConf(ctx)
	if err != nil {
		return errors.Wrap(err, "getting OAuth config")
	}
	token, err := conf.Exchange(ctx, code)
	if err != nil {
		return errors.Wrap(err, "getting OAuth token")
	}
	oauthClient := conf.Client(ctx, token)

	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		return errors.Wrap(err, "creating gmail service client")
	}

	prof, err := gmailSvc.Users.GetProfile("me").Do()
	if err != nil {
		return errors.Wrap(err, "getting gmail profile")
	}

	var (
		u    user
		addr = prof.EmailAddress
	)

	err = aesite.LookupUser(ctx, s.dsClient, addr, &u)
	if errors.Is(err, datastore.ErrNoSuchEntity) {
		u.InboxOnly = true // Force true for now. Later, add a UI for toggling this.
		err = aesite.NewUser(ctx, s.dsClient, addr, "", &u)
		if err != nil {
			return errors.Wrapf(err, "creating user %s", addr)
		}
	} else if err != nil {
		return errors.Wrapf(err, "looking up user %s", addr)
	}

	sess.UserKey = u.Key()
	_, err = s.dsClient.Put(ctx, sess.Key(), sess)
	if err != nil {
		return errors.Wrap(err, "storing session")
	}

	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return errors.Wrap(err, "JSON-marshaling OAuth token")
	}
	u.Token = string(tokenJSON)

	err = s.maybeCreateLabel(ctx, gmailSvc, "✔")
	if err != nil {
		return errors.Wrap(err, "creating ✔ label")
	}
	err = s.maybeCreateLabel(ctx, gmailSvc, "✔/★")
	if err != nil {
		return errors.Wrap(err, "creating ✔/★ label")
	}

	labelsResp, err := gmailSvc.Users.Labels.List("me").Do()
	if err != nil {
		return errors.Wrap(err, "listing labels")
	}
	for _, label := range labelsResp.Labels {
		switch label.Name {
		case "✔":
			u.ContactsLabelID = label.Id
		case "✔/★":
			u.StarredLabelID = label.Id
		}
	}

	_, err = s.dsClient.Put(ctx, u.Key(), &u)
	if err != nil {
		return errors.Wrap(err, "updating user")
	}

	sess.SetCookie(w)

	http.Redirect(w, req, "/", http.StatusSeeOther)

	return nil
}

func (s *Server) maybeCreateLabel(ctx context.Context, gmailSvc *gmail.Service, name string) error {
	label := &gmail.Label{
		LabelListVisibility:   "labelShow",
		MessageListVisibility: "show",
		Name:                  name,
		Type:                  "user",
	}
	_, err := gmailSvc.Users.Labels.Create("me", label).Do()
	if err == nil {
		return nil
	}
	if g, ok := err.(*googleapi.Error); ok {
		switch g.Code {
		case http.StatusConflict, http.StatusNotModified:
			return nil
		}
	}
	return err
}
