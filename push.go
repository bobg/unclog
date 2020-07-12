package unclog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"golang.org/x/time/rate"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
)

const rateLimit = 250 * time.Millisecond

type pushMessage struct {
	Message struct {
		Data string `json:"data"`
	} `json:"message"`
}

type pushPayload struct {
	Addr string `json:"emailAddress"`
}

func (s *Server) handlePush(w http.ResponseWriter, req *http.Request) {
	if !strings.EqualFold(req.Method, "POST") {
		http.Error(w, fmt.Sprintf("method %s not allowed", req.Method), http.StatusMethodNotAllowed)
		return
	}
	if ct := req.Header.Get("Content-Type"); !strings.EqualFold(ct, "application/json") {
		http.Error(w, fmt.Sprintf("content type %s not allowed", ct), http.StatusBadRequest)
		return
	}

	var msg pushMessage
	dec := json.NewDecoder(req.Body)
	err := dec.Decode(&msg)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not JSON-decode request body: %s", err), http.StatusBadRequest)
		return
	}

	decodedData, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not base64-decode request payload: %s", err), http.StatusBadRequest)
		return
	}

	var payload pushPayload
	err = json.Unmarshal(decodedData, &payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("could not JSON-decode request payload: %s", err), http.StatusBadRequest)
		return
	}

	log.Printf("got push for %s", payload.Addr)

	ctx := req.Context()

	oauthConf, err := s.oauthConf(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("configuring oauth: %s", err), http.StatusInternalServerError)
		return
	}

	now := time.Now()
	deadline := now.Add(time.Minute)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var u user
	err = aesite.UpdateUser(ctx, s.dsClient, payload.Addr, &u, func(*datastore.Transaction) error {
		if u.LeaseExpiry.After(now) {
			return errors.New("user is already being processed")
		}
		u.LeaseExpiry = deadline
		return nil
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("locking user %s: %s", payload.Addr, err), http.StatusInternalServerError)
		return
	}
	defer func() {
		aesite.UpdateUser(ctx, s.dsClient, payload.Addr, &u, func(*datastore.Transaction) error {
			u.LeaseExpiry = time.Time{}
			return nil
		})
	}()

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

	oauthClient := oauthConf.Client(ctx, &token)
	limiter := rate.NewLimiter(rate.Every(rateLimit), 1)
	origTransport := oauthClient.Transport
	if origTransport == nil {
		origTransport = http.DefaultTransport
	}
	oauthClient.Transport = &throttledRT{
		rt: origTransport,
		l:  limiter,
	}

	var starred, unstarred []*people.Person

	peopleSvc, err := people.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		http.Error(w, fmt.Sprintf("allocating people service: %s", err), http.StatusInternalServerError)
		return
	}
	peopleConnSvc := people.NewPeopleConnectionsService(peopleSvc)
	err = peopleConnSvc.List("people/me").PersonFields("emailAddresses,names,memberships").Pages(ctx, func(resp *people.ListConnectionsResponse) error {
		for _, person := range resp.Connections {
			var anyAddrs bool
			for _, a := range person.EmailAddresses {
				if a.Value != "" {
					anyAddrs = true
					break
				}
			}
			if !anyAddrs {
				continue
			}
			var isStarred bool
			for _, m := range person.Memberships {
				if m.ContactGroupMembership != nil && m.ContactGroupMembership.ContactGroupId == "starred" {
					isStarred = true
					break
				}
			}
			if isStarred {
				starred = append(starred, person)
			} else {
				unstarred = append(unstarred, person)
			}
		}
		return nil
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("listing connections: %s", err), http.StatusInternalServerError)
		return
	}

	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		http.Error(w, fmt.Sprintf("allocating gmail service: %s", err), http.StatusInternalServerError)
		return
	}

	oneWeekAgo := now.Add(-7 * 24 * time.Hour)
	startTime := u.LastThreadTime
	if startTime.Before(oneWeekAgo) {
		startTime = oneWeekAgo
	}

	query := fmt.Sprintf("after:%d -in:chats", startTime.Unix()-2) // a little overlap, so nothing gets missed
	if u.InboxOnly {
		query += " in:inbox"
	}
	err = gmailSvc.Users.Threads.List("me").Q(query).Pages(ctx, func(resp *gmail.ListThreadsResponse) error {
		for _, thread := range resp.Threads {
			threadTime, err := handleThread(ctx, gmailSvc, &u, thread.Id, starred, unstarred)
			if err != nil {
				return errors.Wrapf(err, "handling thread %s", thread.Id)
			}
			if threadTime.After(u.LastThreadTime) {
				u.LastThreadTime = threadTime
				_, err = s.dsClient.Put(ctx, u.Key(), &u)
				if err != nil {
					return errors.Wrap(err, "storing last-thread time")
				}
			}
		}
		return nil
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("processing latest threads: %s", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func handleThread(ctx context.Context, gmailSvc *gmail.Service, u *user, threadID string, starred, unstarred []*people.Person) (time.Time, error) {
	thread, err := gmailSvc.Users.Threads.Get("me", threadID).Format("metadata").MetadataHeaders("from").Do()
	if err != nil {
		return time.Time{}, errors.Wrap(err, "getting thread members")
	}

	var (
		starredAddr, contactAddr string
		threadTime               time.Time
	)
	for _, msg := range thread.Messages {
		msgTime := timeFromMillis(msg.InternalDate)
		if msgTime.After(threadTime) {
			threadTime = msgTime
		}
		if starredAddr != "" {
			continue
		}
		for _, header := range msg.Payload.Headers {
			if !strings.EqualFold(header.Name, "From") {
				continue
			}
			parsed, err := mail.ParseAddress(header.Value)
			if err != nil {
				log.Printf("skipping message with unparseable From address %s: %s", header.Value, err)
				continue
			}
			if addrIn(parsed.Address, starred) {
				starredAddr = parsed.Address
			} else if addrIn(parsed.Address, unstarred) {
				contactAddr = parsed.Address
			}
			break
		}
	}
	var req *gmail.ModifyThreadRequest
	if starredAddr != "" {
		req = &gmail.ModifyThreadRequest{
			AddLabelIds:    []string{u.StarredLabelID},
			RemoveLabelIds: []string{u.ContactsLabelID},
		}
	} else if contactAddr != "" {
		req = &gmail.ModifyThreadRequest{
			AddLabelIds:    []string{u.ContactsLabelID},
			RemoveLabelIds: []string{u.StarredLabelID},
		}
	} else {
		req = &gmail.ModifyThreadRequest{
			RemoveLabelIds: []string{u.StarredLabelID, u.ContactsLabelID},
		}
	}
	_, err = gmailSvc.Users.Threads.Modify("me", threadID, req).Do()
	if err != nil && !googleapi.IsNotModified(err) {
		return time.Time{}, errors.Wrap(err, "updating thread")
	}
	return threadTime, nil
}

func addrIn(addr string, persons []*people.Person) bool {
	for _, person := range persons {
		for _, personAddr := range person.EmailAddresses {
			if strings.EqualFold(addr, personAddr.Value) {
				return true
			}
		}
	}
	return false
}

type throttledRT struct {
	rt http.RoundTripper
	l  *rate.Limiter
}

func (t throttledRT) RoundTrip(req *http.Request) (*http.Response, error) {
	err := t.l.Wait(req.Context())
	if err != nil {
		return nil, errors.Wrap(err, "while throttled")
	}
	return t.rt.RoundTrip(req)
}
