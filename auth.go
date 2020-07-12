package unclog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func (s *Server) handleAuth(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	sess, err := aesite.NewSession(ctx, s.dsClient, nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("creating session: %s", err), http.StatusInternalServerError)
		return
	}
	sess.SetCookie(w)

	csrf, err := sess.CSRFToken()
	if err != nil {
		http.Error(w, fmt.Sprintf("creating CSRF token: %s", err), http.StatusInternalServerError)
		return
	}

	conf, err := s.oauthConf(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting OAuth config: %s", err), http.StatusInternalServerError)
		return
	}
	url := conf.AuthCodeURL(csrf, oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	http.Redirect(w, req, url, http.StatusSeeOther)
}

// This is where the OAuth flow redirects to.
func (s *Server) handleAuth2(w http.ResponseWriter, req *http.Request) {
	var (
		ctx   = req.Context()
		code  = req.FormValue("code")
		state = req.FormValue("state")
	)

	// Validate state.
	sess, err := aesite.GetSession(ctx, s.dsClient, req)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting session: %s", err), http.StatusInternalServerError)
		return
	}
	err = sess.CSRFCheck(state)
	if err != nil {
		http.Error(w, fmt.Sprintf("in CSRF check: %s", err), http.StatusBadRequest)
		return
	}

	conf, err := s.oauthConf(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting OAuth config: %s", err), http.StatusInternalServerError)
		return
	}
	token, err := conf.Exchange(ctx, code)
	if err != nil {
		http.Error(w, fmt.Sprintf("getting OAuth token: %s", err), http.StatusInternalServerError)
		return
	}

	oauthClient := conf.Client(ctx, token)
	// TODO: rate limiting?

	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		http.Error(w, fmt.Sprintf("creating gmail service client: %s", err), http.StatusInternalServerError)
		return
	}

	prof, err := gmailSvc.Users.GetProfile("me").Do()
	if err != nil {
		http.Error(w, fmt.Sprintf("getting gmail profile: %s", err), http.StatusInternalServerError)
		return
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
			http.Error(w, fmt.Sprintf("creating user %s: %s", addr, err), http.StatusInternalServerError)
			return
		}
	} else if err != nil {
		http.Error(w, fmt.Sprintf("looking up user %s: %s", addr, err), http.StatusInternalServerError)
		return
	}

	sess.UserKey = u.Key()
	_, err = s.dsClient.Put(ctx, sess.Key(), sess)
	if err != nil {
		http.Error(w, fmt.Sprintf("storing session: %s", err), http.StatusInternalServerError)
		return
	}

	tokenJSON, err := json.Marshal(token)
	if err != nil {
		http.Error(w, fmt.Sprintf("JSON-marshaling OAuth token: %s", err), http.StatusInternalServerError)
		return
	}
	u.Token = string(tokenJSON)

	err = s.maybeCreateLabel(ctx, gmailSvc, "Contacts")
	if err != nil {
		http.Error(w, fmt.Sprintf("creating Contacts label: %s", err), http.StatusInternalServerError)
		return
	}
	err = s.maybeCreateLabel(ctx, gmailSvc, "Contacts/Starred")
	if err != nil {
		http.Error(w, fmt.Sprintf("creating Contacts/Starred label: %s", err), http.StatusInternalServerError)
		return
	}

	labelsResp, err := gmailSvc.Users.Labels.List("me").Do()
	if err != nil {
		http.Error(w, fmt.Sprintf("listing labels: %s", err), http.StatusInternalServerError)
		return
	}
	for _, label := range labelsResp.Labels {
		switch label.Name {
		case "Contacts":
			u.ContactsLabelID = label.Id
		case "Contacts/Starred":
			u.StarredLabelID = label.Id
		}
	}

	_, err = s.dsClient.Put(ctx, u.Key(), &u)
	if err != nil {
		http.Error(w, fmt.Sprintf("updating user: %s", err), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, req, "/", http.StatusSeeOther)
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
