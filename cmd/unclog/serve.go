package main

import (
	"context"
	"flag"
	"fmt"

	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"github.com/pkg/errors"
	"google.golang.org/api/option"

	"github.com/bobg/unclog"
)

func cliServe(ctx context.Context, flagset *flag.FlagSet, args []string) error {
	var (
		creds     = flagset.String("creds", "", "credentials file")
		projectID = flagset.String("project", "", "project ID")
		test      = flagset.Bool("test", false, "run in test mode")
	)

	err := flagset.Parse(args)
	if err != nil {
		return err
	}

	if *test {
		if *creds != "" {
			return fmt.Errorf("cannot supply both -test and -creds")
		}

		err := aesite.DSTest(ctx, *projectID)
		if err != nil {
			return errors.Wrap(err, "starting test datastore service")
		}
	}

	var options []option.ClientOption
	if *creds != "" {
		options = append(options, option.WithCredentialsFile(*creds))
	}
	dsClient, err := datastore.NewClient(ctx, *projectID, options...)
	if err != nil {
		return errors.Wrap(err, "creating datastore client")
	}

	s := unclog.NewServer(dsClient)
	err = s.Serve(ctx)

	return errors.Wrap(err, "running server")
}
