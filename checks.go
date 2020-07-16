package unclog

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bobg/aesite"
	"github.com/bobg/mid"
	"github.com/pkg/errors"
	"google.golang.org/appengine"
)

var (
	errNoKey    = errors.New("no key field supplied")
	errWrongKey = errors.New("wrong key supplied")
)

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

// See
// https://cloud.google.com/tasks/docs/creating-appengine-handlers#reading_request_headers.
func (s *Server) checkTaskQueue(req *http.Request) error {
	if !appengine.IsAppEngine() {
		return nil
	}

	err := s.checkMasterKey(req)
	if err == nil { // sic
		return nil
	}

	h := strings.TrimSpace(req.Header.Get("X-AppEngine-QueueName"))
	if h != s.queueName() {
		return mid.CodeErr{
			C:   http.StatusUnauthorized,
			Err: fmt.Errorf("header value %s does not match queue name %s", h, s.queueName()),
		}
	}
	return nil
}
