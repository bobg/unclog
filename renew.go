package unclog

import (
	stderrs "errors"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/datastore"
	"github.com/pkg/errors"
	"google.golang.org/api/iterator"
)

func (s *Server) handleRenew(w http.ResponseWriter, req *http.Request) error {
	ctx := req.Context()

	var (
		now       = time.Now()
		yesterday = now.Add(-24 * time.Hour)
		tomw      = now.Add(24 * time.Hour)
	)
	q := datastore.NewQuery("User").Filter("WatchExpiry >", yesterday).Filter("WatchExpiry <", tomw)
	it := s.dsClient.Run(ctx, q)
	for {
		var u user
		_, err := it.Next(&u)
		if stderrs.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return errors.Wrap(err, "iterating over users")
		}

		err = s.watch(ctx, &u)
		if err != nil {
			log.Printf("renewing gmail watch for %s: %s", u.Email, err)
		}
		log.Printf("renewed gmail watch for %s, new expiry %s", u.Email, u.WatchExpiry)
	}

	return nil
}
