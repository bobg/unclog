package unclog

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bobg/aesite"
	"github.com/bobg/mid"
	"github.com/pkg/errors"
	"google.golang.org/api/idtoken"
	"google.golang.org/appengine"
)

var (
	errNoKey    = errors.New("no key field supplied")
	errWrongKey = errors.New("wrong key supplied")
)

// Read the master key from settings.
func (s *Server) getMasterKey(ctx context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.masterKey == "" {
		masterKey, err := aesite.GetSetting(ctx, s.dsClient, "master-key")
		if err != nil {
			return "", err
		}
		s.masterKey = string(masterKey)
	}
	return s.masterKey, nil
}

// Check that a request has the right value for X-Unclog-Key.
// This can be used to bypass other checks.
func (s *Server) checkMasterKey(req *http.Request) error {
	key := req.Header.Get("X-Unclog-Key")
	if key == "" {
		return errNoKey
	}
	key = strings.TrimSpace(key)

	masterKey, err := s.getMasterKey(req.Context())
	if err != nil {
		return err
	}

	if key != masterKey {
		return errWrongKey
	}
	return nil
}

func (s *Server) checkCron(req *http.Request) error {
	if !appengine.IsAppEngine() {
		return nil
	}
	err := s.checkMasterKey(req)
	if err == nil { // sic
		return nil
	}
	h := strings.TrimSpace(req.Header.Get("X-Appengine-Cron"))
	if h != "true" {
		return mid.CodeErr{C: http.StatusUnauthorized}
	}
	return nil
}

// Check that the request comes from the Google task queue.
// (See
// https://cloud.google.com/tasks/docs/creating-appengine-handlers#reading_app_engine_task_request_headers.)
//
// This is a no-op outside of the App Engine context.
//
// It can be bypassed with the right X-Unclog-Key header field.
// See checkMasterKey, above.
func (s *Server) checkTaskQueue(req *http.Request) error {
	if !appengine.IsAppEngine() {
		return nil
	}

	err := s.checkMasterKey(req)
	if err == nil { // sic
		return nil
	}

	h := strings.TrimSpace(req.Header.Get("X-AppEngine-QueueName"))
	if h != queueName {
		return mid.CodeErr{
			C:   http.StatusUnauthorized,
			Err: fmt.Errorf("header value %s does not match queue name %s", h, s.queueName()),
		}
	}
	return nil
}

// Check that the request contains a valid Authorization field.
// This is expected to be present in the pubsub notification sent by the Gmail watcher.
//
// It can be bypassed with the right X-Unclog-Key header field.
// See checkMasterKey, above.
func (s *Server) checkAuthHeader(req *http.Request) error {
	err := s.checkMasterKey(req)
	if err == nil { // sic
		return nil
	}
	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		return errors.New("no Authorization field")
	}
	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 {
		return fmt.Errorf("Authorization field has %d part(s), want 2", len(parts))
	}
	if !strings.EqualFold(parts[0], "Bearer") {
		return fmt.Errorf("Authorization type is %s, want Bearer", parts[0])
	}
	tok := parts[1]
	_, err = idtoken.Validate(req.Context(), tok, "")
	return errors.Wrap(err, "validating Authorization token")
}
