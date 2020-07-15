package unclog

import (
	"context"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

func (s *Server) watch(ctx context.Context, u *user) error {
	return s.watchHelper(ctx, u, true)
}

func (s *Server) stop(ctx context.Context, u *user) error {
	return s.watchHelper(ctx, u, false)
}

func (s *Server) watchHelper(ctx context.Context, u *user, watch bool) error {
	client, err := s.oauthClient(ctx, u)
	if err != nil {
		return errors.Wrap(err, "getting oauth client")
	}

	gmailSvc, err := gmail.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return errors.Wrap(err, "allocating gmail service")
	}

	if watch {
		watchReq := &gmail.WatchRequest{
			TopicName: pubsubTopic,
		}
		watchResp, err := gmailSvc.Users.Watch("me", watchReq).Do()
		if err != nil {
			return errors.Wrap(err, "subscribing to gmail push notices")
		}
		u.WatchExpiry = timeFromMillis(watchResp.Expiration)
	} else {
		err := gmailSvc.Users.Stop("me").Do()
		if err != nil {
			return errors.Wrap(err, "unsubscribing from gmail push notices")
		}
		u.WatchExpiry = time.Time{}
	}

	_, err = s.dsClient.Put(ctx, u.Key(), &u)
	return errors.Wrap(err, "updating user")
}
