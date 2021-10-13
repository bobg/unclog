package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"github.com/pkg/errors"
	"google.golang.org/api/option"

	"github.com/bobg/unclog"
)

func cliServe(ctx context.Context, creds, projectID, locationID, contentDir string, test bool, _ []string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		sig := <-sigCh
		log.Printf("got signal %s", sig)
		cancel()
	}()

	if test {
		if creds != "" {
			return fmt.Errorf("cannot supply both -test and -creds")
		}

		err := aesite.DSTest(ctx, projectID)
		if err != nil {
			return errors.Wrap(err, "starting test datastore service")
		}
	}

	var options []option.ClientOption
	if creds != "" {
		options = append(options, option.WithCredentialsFile(creds))
	}
	dsClient, err := datastore.NewClient(ctx, projectID, options...)
	if err != nil {
		return errors.Wrap(err, "creating datastore client")
	}
	ctClient, err := cloudtasks.NewClient(ctx, options...)
	if err != nil {
		return errors.Wrap(err, "creating cloudtasks client")
	}

	s := unclog.NewServer(dsClient, ctClient, projectID, locationID, contentDir)
	err = s.Serve(ctx)

	return errors.Wrap(err, "running server")
}
