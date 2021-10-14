package main

import (
	"context"
	"flag"
	"log"
	"os"

	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"github.com/bobg/subcmd/v2"
	"github.com/pkg/errors"
	"google.golang.org/api/option"
	"google.golang.org/appengine"
)

const (
	defaultProject = "unclog"
	defaultRegion  = "us-west2"
	defaultDir     = "web/build"
)

func main() {
	var (
		creds     = flag.String("creds", "", "credentials file")
		projectID = flag.String("project", defaultProject, "project ID")
		test      = flag.Bool("test", false, "run in test mode")
	)
	flag.Parse()

	if *test && *creds != "" {
		log.Fatal("Cannot supply both -test and -creds")
	}

	if appengine.IsAppEngine() && len(os.Args) < 2 {
		err := doServe(context.Background(), *creds, *projectID, defaultRegion, defaultDir, *test)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		c := maincmd{creds: *creds, projectID: *projectID, test: *test}
		err := subcmd.Run(context.Background(), c, flag.Args())
		if err != nil {
			log.Fatal(err)
		}
	}
}

type maincmd struct {
	creds     string
	projectID string
	test      bool
}

func (c maincmd) Subcmds() subcmd.Map {
	return subcmd.Commands(
		"admin", c.cliAdmin, "perform admin tasks", nil,
		"serve", c.cliServe, "run a server", subcmd.Params(
			"-location", subcmd.String, defaultRegion, "location ID",
			"-dir", subcmd.String, defaultDir, "content dir",
		),
	)
}

func (c maincmd) dsClient(ctx context.Context) (*datastore.Client, error) {
	return getDSClient(ctx, c.creds, c.projectID, c.test)
}

func getDSClient(ctx context.Context, creds, projectID string, test bool) (*datastore.Client, error) {
	if test {
		err := aesite.DSTest(ctx, projectID)
		if err != nil {
			return nil, errors.Wrap(err, "starting test datastore")
		}
	}
	var options []option.ClientOption
	if creds != "" {
		options = append(options, option.WithCredentialsFile(creds))
	}
	return datastore.NewClient(ctx, projectID, options...)
}
