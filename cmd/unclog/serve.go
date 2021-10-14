package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"github.com/pkg/errors"
	"google.golang.org/api/option"

	"github.com/bobg/unclog"
)

func (c maincmd) cliServe(ctx context.Context, locationID, contentDir string, test bool, _ []string) error {
	return doServe(ctx, c.creds, c.projectID, locationID, contentDir, test)
}

func doServe(ctx context.Context, creds, projectID, locationID, contentDir string, test bool) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	dsClient, err := getDSClient(ctx, creds, projectID, test)
	if err != nil {
		return errors.Wrap(err, "creating datastore client")
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		sig := <-sigCh
		log.Printf("got signal %s", sig)
		cancel()
	}()

	var options []option.ClientOption
	if creds != "" {
		options = append(options, option.WithCredentialsFile(creds))
	}
	ctClient, err := cloudtasks.NewClient(ctx, options...)
	if err != nil {
		return errors.Wrap(err, "creating cloudtasks client")
	}

	s := unclog.NewServer(dsClient, ctClient, projectID, locationID, contentDir)
	err = s.Serve(ctx)

	return errors.Wrap(err, "running server")
}
