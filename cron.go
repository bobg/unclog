package unclog

import (
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/pkg/errors"
	"google.golang.org/api/iterator"
)

// GET/POST /t/cron
func (s *Server) handleCron(_ http.ResponseWriter, req *http.Request) error {
	err := s.checkCron(req)
	if err != nil {
		return err
	}

	// Part 1: renew any gmail watches that are close to expiration.

	var (
		ctx       = req.Context()
		now       = time.Now()
		yesterday = now.Add(-24 * time.Hour)
		tomw      = now.Add(24 * time.Hour)
	)
	q := datastore.NewQuery("User").Filter("WatchExpiry >", yesterday).Filter("WatchExpiry <", tomw)
	it := s.dsClient.Run(ctx, q)
	for {
		var u user
		_, err := it.Next(&u)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return errors.Wrap(err, "iterating over users for renew")
		}

		err = s.watch(ctx, &u)
		if err != nil {
			log.Printf("renewing gmail watch for %s: %s", u.Email, err)
		} else {
			log.Printf("renewed gmail watch for %s, new expiry %s", u.Email, u.WatchExpiry)
		}
	}

	// Part 2: queue catch-up updates for addresses with no update in the past 24 hours.
	// Maybe pubsub notifications stopped arriving, which is a thing that happens sometimes.

	q = datastore.NewQuery("User").Filter("LastUpdate <", yesterday)
	it = s.dsClient.Run(ctx, q)
	for {
		var u user
		_, err := it.Next(&u)
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return errors.Wrap(err, "iterating over users for update")
		}

		err = s.queueUpdate(ctx, u.Email, "", true)
		if err != nil {
			log.Printf("queueing catch-up update for %s: %s", u.Email, err)
		} else {
			log.Printf("queued catch-up update for %s", u.Email)
		}
	}

	return nil
}
