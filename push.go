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
	"github.com/bobg/mid"
	"github.com/pkg/errors"
	"golang.org/x/oauth2"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
)

type PushMessage struct {
	Message struct {
		Data string `json:"data"`
	} `json:"message"`
	Date string `json:"date,omitempty"`
}

type PushPayload struct {
	Addr string `json:"emailAddress"`
}

func (s *Server) handlePush(w http.ResponseWriter, req *http.Request) (err error) {
	s.pushCalls.Add(1)
	begin := time.Now()
	defer func() {
		elapsed := time.Since(begin)
		if err != nil {
			s.pushErrs.Add(1)
		} else {
			s.pushCumSecs.Add(float64(elapsed) / float64(time.Second))
		}
	}()

	if !strings.EqualFold(req.Method, "POST") {
		return mid.CodeErr{C: http.StatusMethodNotAllowed}
	}
	if ct := req.Header.Get("Content-Type"); !strings.EqualFold(ct, "application/json") {
		return mid.CodeErr{C: http.StatusBadRequest, Err: fmt.Errorf("content type %s not allowed", ct)}
	}

	var msg PushMessage
	dec := json.NewDecoder(req.Body)
	err = dec.Decode(&msg)
	if err != nil {
		return mid.CodeErr{C: http.StatusBadRequest, Err: errors.Wrap(err, "JSON-decoding request body")}
	}

	decodedData, err := base64.StdEncoding.DecodeString(msg.Message.Data)
	if err != nil {
		return mid.CodeErr{C: http.StatusBadRequest, Err: errors.Wrap(err, "base64-decoding request payload")}
	}

	var payload PushPayload
	err = json.Unmarshal(decodedData, &payload)
	if err != nil {
		return mid.CodeErr{C: http.StatusBadRequest, Err: errors.Wrap(err, "JSON-decoding request payload")}
	}

	log.Printf("got push for %s", payload.Addr)

	ctx := req.Context()

	oauthConf, err := s.oauthConf(ctx)
	if err != nil {
		return errors.Wrap(err, "configuring oauth")
	}

	now := time.Now()
	deadline := now.Add(time.Minute)
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var u user
	err = aesite.UpdateUser(ctx, s.dsClient, payload.Addr, &u, func(*datastore.Transaction) error {
		if u.LeaseExpiry.After(now) {
			return fmt.Errorf("lease will expire at %s", u.LeaseExpiry)
		}
		u.LeaseExpiry = deadline
		return nil
	})
	if err != nil {
		s.pushCollisions.Add(1)
		log.Printf("locking user %s: %s", payload.Addr, err)
		return nil
	}
	defer func() {
		aesite.UpdateUser(ctx, s.dsClient, payload.Addr, &u, func(*datastore.Transaction) error {
			u.LeaseExpiry = time.Time{}
			return nil
		})
	}()

	if u.Token == "" {
		// xxx cancel session?
		return mid.CodeErr{C: http.StatusUnauthorized}
	}
	var token oauth2.Token
	err = json.Unmarshal([]byte(u.Token), &token)
	if err != nil {
		return errors.Wrap(err, "decoding oauth token")
	}

	oauthClient := oauthConf.Client(ctx, &token)

	var starred, unstarred []*people.Person

	peopleSvc, err := people.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		return errors.Wrap(err, "allocating people service")
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
		return errors.Wrap(err, "listing connections")
	}

	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(oauthClient))
	if err != nil {
		return errors.Wrap(err, "allocating gmail service")
	}

	var query string
	if u.InboxOnly {
		query = "in:inbox"
	} else {
		query = "-in:chats"
	}
	if msg.Date != "" {
		d, err := ParseDate(msg.Date)
		if err != nil {
			return errors.Wrap(err, "parsing date")
		}
		query += fmt.Sprintf(" after:%d/%02d/%02d", d.Y, d.M, d.D)
		d = nextDate(d)
		query += fmt.Sprintf(" before:%d/%02d/%02d", d.Y, d.M, d.D)
	} else {
		oneWeekAgo := now.Add(-7 * 24 * time.Hour)
		startTime := u.LastThreadTime
		if startTime.Before(oneWeekAgo) {
			startTime = oneWeekAgo
		}
		query += fmt.Sprintf(" after:%d", startTime.Unix()-2) // a little overlap, so nothing gets missed
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

	return errors.Wrap(err, "processing latest threads")
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
			// No need to keep looking for addresses,
			// but do keep iterating over messages
			// to get an accurate value for threadTime.
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
	}
	if req != nil {
		_, err = gmailSvc.Users.Threads.Modify("me", threadID, req).Do()
		if err != nil && !googleapi.IsNotModified(err) {
			return time.Time{}, errors.Wrap(err, "updating thread")
		}
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
