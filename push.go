package unclog

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	stderrs "errors"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"github.com/bobg/basexx"
	"github.com/bobg/mid"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/pkg/errors"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
	"google.golang.org/genproto/googleapis/cloud/tasks/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	defer func() {
		if err != nil {
			log.Printf("ERROR %s", err)
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

	now := time.Now()

	var (
		u    user
		when time.Time
	)
	err = aesite.UpdateUser(ctx, s.dsClient, payload.Addr, &u, func(*datastore.Transaction) error {
		if u.NextUpdate.After(now) {
			when = u.NextUpdate
		} else {
			when = now
			u.NextUpdate = now.Add(time.Minute)
		}
		return nil
	})
	if err != nil && !stderrs.Is(err, aesite.ErrUpdateConflict) { // OK to ignore ErrUpdateConflict
		return errors.Wrap(err, "getting user and updating next-update time")
	}

	if u.WatchExpiry.IsZero() {
		log.Printf("ignoring push for disabled user %s", u.Email)
		return nil
	}

	var (
		secs  = when.Unix()
		nanos = int32(when.UnixNano() % int64(time.Second))
	)

	_, err = s.ctClient.CreateTask(ctx, &tasks.CreateTaskRequest{
		Parent: s.queueName(),
		Task: &tasks.Task{
			Name: s.taskName(u.Email, when),
			MessageType: &tasks.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &tasks.AppEngineHttpRequest{
					HttpMethod:  tasks.HttpMethod_GET,
					RelativeUri: s.taskURL(u.Email, msg.Date),
				},
			},
			ScheduleTime: &timestamp.Timestamp{
				Seconds: secs,
				Nanos:   nanos,
			},
		},
	})
	if status.Code(err) == codes.AlreadyExists {
		log.Printf("deduped update task for %s at %s", u.Email, when)
	} else if err != nil {
		return errors.Wrapf(err, "enqueueing update task for %s at %s", u.Email, when)
	}

	return nil
}

func (s *Server) taskName(email string, when time.Time) string {
	hasher := sha256.New()
	hasher.Write([]byte{1}) // version of this hash
	fmt.Fprintf(hasher, "%s %s", email, when)
	h := hasher.Sum(nil)
	src := basexx.NewBuffer(h, basexx.Binary)
	buf := make([]byte, basexx.Length(256, 50, len(h)))
	dest := basexx.NewBuffer(buf[:], basexx.Base50)
	_, err := basexx.Convert(dest, src)
	if err != nil {
		panic(err)
	}
	converted := dest.Written()
	return fmt.Sprintf("%s/tasks/%s", s.queueName(), string(converted))
}

const queueName = "update"

func (s *Server) queueName() string {
	return fmt.Sprintf("projects/%s/locations/%s/queues/%s", s.projectID, s.locationID, queueName)
}

func (s *Server) taskURL(email, date string) string {
	u, _ := url.Parse("/t/update")

	v := url.Values{}
	v.Set("email", email)
	if date != "" {
		v.Set("date", date)
	}
	u.RawQuery = v.Encode()

	return u.String()
}

func (s *Server) handleUpdate(w http.ResponseWriter, req *http.Request) (err error) {
	defer func() {
		if err != nil {
			log.Printf("ERROR %s", err)
		}
	}()

	err = s.checkTaskQueue(req)
	if err != nil {
		return err
	}

	var (
		ctx   = req.Context()
		date  = req.FormValue("date")
		email = req.FormValue("email")
		now   = time.Now()
	)

	var u user
	err = aesite.UpdateUser(ctx, s.dsClient, email, &u, func(*datastore.Transaction) error {
		nextUpdate := now.Add(time.Minute)
		if nextUpdate.After(u.NextUpdate) {
			u.NextUpdate = nextUpdate
		}
		return nil
	})

	oauthClient, err := s.oauthClient(ctx, &u) // xxx check for errNoToken
	if err != nil {
		return errors.Wrap(err, "getting oauth client")
	}

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
	if date != "" {
		d, err := ParseDate(date)
		if err != nil {
			return errors.Wrap(err, "parsing date")
		}
		query += fmt.Sprintf(" after:%d/%02d/%02d", d.Y, d.M, d.D)
		d = nextDate(d)
		query += fmt.Sprintf(" before:%d/%02d/%02d", d.Y, d.M, d.D)
	} else {
		var (
			oneWeekAgo = now.Add(-7 * 24 * time.Hour)
			startTime  = u.LastThreadTime.Add(-5 * time.Second) // a little overlap, so nothing gets missed
		)
		if startTime.Before(oneWeekAgo) {
			startTime = oneWeekAgo
		}
		query += fmt.Sprintf(" after:%d", startTime.Unix())
	}

	var numThreads, numStarred, numUnstarred int

	err = gmailSvc.Users.Threads.List("me").Q(query).Pages(ctx, func(resp *gmail.ListThreadsResponse) error {
		for _, thread := range resp.Threads {
			threadTime, outcome, err := handleThread(ctx, gmailSvc, &u, thread.Id, starred, unstarred)
			if err != nil {
				return errors.Wrapf(err, "handling thread %s", thread.Id)
			}
			numThreads++
			switch outcome {
			case outcomeStarred:
				numStarred++
			case outcomeUnstarred:
				numUnstarred++
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
		return errors.Wrap(err, "processing latest threads")
	}

	log.Printf("processing %s: %d thread(s), %d starred, %d unstarred", u.Email, numThreads, numStarred, numUnstarred)
	return nil
}

type outcome int

const (
	outcomeNone outcome = iota
	outcomeStarred
	outcomeUnstarred
)

func handleThread(ctx context.Context, gmailSvc *gmail.Service, u *user, threadID string, starred, unstarred []*people.Person) (time.Time, outcome, error) {
	thread, err := gmailSvc.Users.Threads.Get("me", threadID).Format("metadata").MetadataHeaders("from").Do()
	if err != nil {
		return time.Time{}, outcomeNone, errors.Wrap(err, "getting thread members")
	}

	var (
		starredAddr, unstarredAddr   string
		foundStarred, foundUnstarred bool
		threadTime                   time.Time
	)
	for _, msg := range thread.Messages {
		msgTime := timeFromMillis(msg.InternalDate)
		if msgTime.After(threadTime) {
			threadTime = msgTime
		}
		if !foundStarred || !foundUnstarred {
			for _, labelID := range msg.LabelIds {
				switch labelID {
				case u.StarredLabelID:
					foundStarred = true
				case u.ContactsLabelID:
					foundUnstarred = true
				}
			}
		}
		if starredAddr != "" {
			// No need to keep looking for addresses,
			// but do keep iterating over messages
			// to get accurate values for threadTime,
			// foundStarred, and foundUnstarred.
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
				unstarredAddr = parsed.Address
			}
			break
		}
	}
	var (
		req *gmail.ModifyThreadRequest
		o   = outcomeNone
	)

	u.NumThreads++

	if starredAddr != "" && !foundStarred {
		u.NumLabeled++
		req = &gmail.ModifyThreadRequest{
			AddLabelIds:    []string{u.StarredLabelID},
			RemoveLabelIds: []string{u.ContactsLabelID},
		}
		o = outcomeStarred
	} else if unstarredAddr != "" && !foundUnstarred {
		u.NumLabeled++
		req = &gmail.ModifyThreadRequest{
			AddLabelIds:    []string{u.ContactsLabelID},
			RemoveLabelIds: []string{u.StarredLabelID},
		}
		o = outcomeUnstarred
	} else if foundStarred || foundUnstarred {
		// Thread is labeled but should not be.
		// (Maybe someone was removed from the user's contacts?)
		req = &gmail.ModifyThreadRequest{
			RemoveLabelIds: []string{u.StarredLabelID, u.ContactsLabelID},
		}
	}
	if req != nil {
		_, err = gmailSvc.Users.Threads.Modify("me", threadID, req).Do()
		if err != nil && !googleapi.IsNotModified(err) {
			return time.Time{}, outcomeNone, errors.Wrap(err, "updating thread")
		}
	}
	return threadTime, o, nil
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
