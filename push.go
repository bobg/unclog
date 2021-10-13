package unclog

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/mail"
	"net/url"
	"strconv"
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

// PushMessage is for parsing the message delivered by the gmail pubsub notification.
type PushMessage struct {
	Message struct {
		Data string `json:"data"`
	} `json:"message"`
	Date string `json:"date,omitempty"`
}

// PushPayload is for parsing the content of PushMessage.Message.Data after decoding.
type PushPayload struct {
	Addr string `json:"emailAddress"`
}

// POST /push
func (s *Server) handlePush(_ http.ResponseWriter, req *http.Request) (err error) {
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

	err = s.checkAuthHeader(req)
	if err != nil {
		return mid.CodeErr{C: http.StatusUnauthorized, Err: err}
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

	err = s.queueUpdate(req.Context(), payload.Addr, msg.Date, false)
	if errors.Is(err, datastore.ErrNoSuchEntity) {
		log.Printf("ignoring push for unknown user %s", payload.Addr)
		return nil
	}
	return errors.Wrap(err, "queueing update")
}

// Queue an update task for the user with address `email`.
// Optional `date` (yyyy-mm-dd) says at what date to begin scanning mail; default is one week ago.
//
// If NextUpdate is set and in the future, the task is scheduled for then
// (and possibly deduped with any other task scheduled for the same time).
// Otherwise, the task is scheduled for now and NextUpdate is set to a minute from now.
// This prevents multiple pushes arriving in the same minute from producing more than one task.
//
// If isCatchup is true, this is a "catch-up" update.
// The cron job queues a catch-up update when there has been no other update for over a day
// (which could mean that pubsub notifications have prematurely stopped,
// which appears to be a thing that happens).
//
// TODO: NextUpdate doesn't actually mean that there's an update scheduled for then;
// rename it to something like NoUpdatesBefore.
func (s *Server) queueUpdate(ctx context.Context, email, date string, isCatchup bool) error {
	var (
		now  = time.Now()
		u    user
		when time.Time
	)
	err := aesite.UpdateUser(ctx, s.dsClient, email, &u, func(*datastore.Transaction) error {
		if u.NextUpdate.After(now) {
			when = u.NextUpdate
		} else {
			when = now
			u.NextUpdate = now.Add(time.Minute)
		}
		return nil
	})
	if err != nil && !errors.Is(err, aesite.ErrUpdateConflict) { // OK to ignore ErrUpdateConflict
		return errors.Wrapf(err, "getting user %s and updating next-update time", email)
	}

	if u.WatchExpiry.IsZero() {
		log.Printf("not queueing update for disabled user %s", email)
		return nil
	}

	var (
		secs  = when.Unix()
		nanos = int32(when.UnixNano() % int64(time.Second))
	)

	_, err = s.ctClient.CreateTask(ctx, &tasks.CreateTaskRequest{
		Parent: s.queueName(),
		Task: &tasks.Task{
			Name: s.taskName(email, when),
			MessageType: &tasks.Task_AppEngineHttpRequest{
				AppEngineHttpRequest: &tasks.AppEngineHttpRequest{
					HttpMethod:  tasks.HttpMethod_GET,
					RelativeUri: s.taskURL(email, date, isCatchup),
				},
			},
			ScheduleTime: &timestamp.Timestamp{
				Seconds: secs,
				Nanos:   nanos,
			},
		},
	})
	if status.Code(err) == codes.AlreadyExists {
		log.Printf("deduped update task for %s at %s", email, when)
		return nil
	}
	return errors.Wrapf(err, "enqueueing update task for %s at %s", email, when)
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

func (s *Server) taskURL(email, date string, isCatchup bool) string {
	u, _ := url.Parse("/t/update")

	v := url.Values{}
	v.Set("email", email)
	if date != "" {
		v.Set("date", date)
	}
	if isCatchup {
		v.Set("catchup", "true")
	}
	u.RawQuery = v.Encode()

	return u.String()
}

// GET/POST /t/update
func (s *Server) handleUpdate(_ http.ResponseWriter, req *http.Request) (err error) {
	defer func() {
		if err != nil {
			log.Printf("ERROR %s", err)
		}
	}()

	err = s.checkTaskQueue(req)
	if err != nil {
		return err
	}

	isCatchup, _ := strconv.ParseBool(req.FormValue("catchup"))

	return s.doUpdate(req.Context(), req.FormValue("email"), req.FormValue("date"), isCatchup)
}

// Executes an update task.
// This adds and removes labels for e-mail newer than `date` (default: one week ago)
// for the user with address `email`.
//
// If `isCatchup` is true, and changes were needed,
// this is a signal that pubsub notifications have prematurely stopped arriving.
// In that case we try to renew the pubsub subscription.
//
// This function updates the NextUpdate and LastUpdate times for the user.
func (s *Server) doUpdate(ctx context.Context, email, date string, isCatchup bool) error {
	var (
		now = time.Now()
		u   user
	)
	err := aesite.UpdateUser(ctx, s.dsClient, email, &u, func(*datastore.Transaction) error {
		nextUpdate := now.Add(time.Minute)
		if nextUpdate.After(u.NextUpdate) {
			u.NextUpdate = nextUpdate
		}
		u.LastUpdate = now
		return nil
	})
	if err != nil {
		return errors.Wrapf(err, "setting NextUpdate and LastUpdate for %s", email)
	}

	oauthClient, err := s.oauthClient(ctx, &u) // xxx check for errNoToken
	if err != nil {
		return errors.Wrap(err, "getting oauth client")
	}

	// Part 1: get user's contacts.

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

	// Part 2: process messages in the right time range.

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

	var (
		nchanges         int
		latestThreadTime = u.LastThreadTime
	)

	err = gmailSvc.Users.Threads.List("me").Q(query).Pages(ctx, func(resp *gmail.ListThreadsResponse) error {
		for _, thread := range resp.Threads {
			threadTime, changed, err := handleThread(ctx, gmailSvc, thread.Id, u.StarredLabelID, u.ContactsLabelID, starred, unstarred)
			if err != nil {
				return errors.Wrapf(err, "handling thread %s", thread.Id)
			}
			if changed {
				nchanges++
			}
			if threadTime.After(latestThreadTime) {
				latestThreadTime = threadTime
			}
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "processing latest threads")
	}

	if latestThreadTime.After(u.LastThreadTime) {
		err = aesite.UpdateUser(ctx, s.dsClient, email, &u, func(*datastore.Transaction) error {
			// Recheck the outer condition to prevent races.
			if latestThreadTime.After(u.LastThreadTime) {
				u.LastThreadTime = latestThreadTime
			}
			return nil
		})
		if err != nil && !errors.Is(err, aesite.ErrUpdateConflict) { // OK to ignore ErrUpdateConflict
			return errors.Wrapf(err, "updating LastThreadTime for %s", email)
		}
	}

	if nchanges > 0 {
		log.Printf("marked/unmarked %d thread(s)", nchanges)
		if isCatchup {
			err = s.watch(ctx, &u)
			if err != nil {
				return errors.Wrapf(err, "renewing gmail watch for %s", u.Email)
			}
			log.Printf("renewed gmail watch in catchup update for %s", u.Email)
		}
	}

	return nil
}

// Add/remove labels on the messages in a given thread.
// Returns the timestamp of the latest message in the thread
// and a boolean telling whether any change was made.
func handleThread(ctx context.Context, gmailSvc *gmail.Service, threadID string, starredLabelID, unstarredLabelID string, starred, unstarred []*people.Person) (time.Time, bool, error) {
	var threadTime time.Time

	thread, err := gmailSvc.Users.Threads.Get("me", threadID).Format("metadata").MetadataHeaders("from").Do()
	if err != nil {
		return threadTime, false, errors.Wrap(err, "getting thread members")
	}

	var (
		starredAddr, unstarredAddr   string
		foundStarred, foundUnstarred bool
	)

	for _, msg := range thread.Messages {
		msgTime := timeFromMillis(msg.InternalDate)
		if msgTime.After(threadTime) {
			threadTime = msgTime
		}
		if !foundStarred || !foundUnstarred {
			for _, labelID := range msg.LabelIds {
				switch labelID {
				case starredLabelID:
					foundStarred = true
				case unstarredLabelID:
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

	var req *gmail.ModifyThreadRequest

	if starredAddr != "" {
		if !foundStarred {
			req = &gmail.ModifyThreadRequest{
				AddLabelIds:    []string{starredLabelID},
				RemoveLabelIds: []string{unstarredLabelID},
			}
		}
	} else if unstarredAddr != "" {
		if !foundUnstarred {
			req = &gmail.ModifyThreadRequest{
				AddLabelIds:    []string{unstarredLabelID},
				RemoveLabelIds: []string{starredLabelID},
			}
		}
	} else if foundStarred || foundUnstarred {
		// Thread is labeled but should not be.
		// (Maybe someone was removed from the user's contacts?)
		req = &gmail.ModifyThreadRequest{
			RemoveLabelIds: []string{starredLabelID, unstarredLabelID},
		}
	}

	var change bool
	if req != nil {
		_, err = gmailSvc.Users.Threads.Modify("me", threadID, req).Do()
		if err != nil && !googleapi.IsNotModified(err) {
			return threadTime, false, errors.Wrap(err, "updating thread")
		}
		change = true
	}

	return threadTime, change, nil
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
