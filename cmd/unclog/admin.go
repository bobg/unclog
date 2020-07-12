package main

import (
	"context"
	"flag"
	"fmt"

	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"github.com/pkg/errors"
	"google.golang.org/api/option"
)

var adminCommands = map[string]func(context.Context, *flag.FlagSet, []string) error{
	"get": cliAdminGet,
	"set": cliAdminSet,
}

func cliAdmin(ctx context.Context, flagset *flag.FlagSet, args []string) error {
	err := flagset.Parse(args)
	if err != nil {
		return err
	}

	if flagset.NArg() == 0 {
		return errors.New("usage: unclog admin <subcommand> [args]")
	}

	cmd := flagset.Arg(0)
	fn, ok := adminCommands[cmd]
	if !ok {
		return fmt.Errorf("unknown admin subcommand %s", cmd)
	}

	args = flagset.Args()
	return fn(ctx, flag.NewFlagSet("", flag.ContinueOnError), args[1:])
}

func cliAdminGet(ctx context.Context, flagset *flag.FlagSet, args []string) error {
	var (
		creds     = flagset.String("creds", "", "credentials file")
		projectID = flagset.String("project", "unclog", "project ID")
		test      = flagset.Bool("test", false, "run in test mode")
	)

	err := flagset.Parse(args)
	if err != nil {
		return err
	}

	if flagset.NArg() != 1 {
		return errors.New("usage: unclog admin get VAR")
	}

	if *test {
		if *creds != "" {
			return fmt.Errorf("cannot supply both -test and -creds")
		}

		err := aesite.DSTest(ctx, *projectID)
		if err != nil {
			return err
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

	val, err := aesite.GetSetting(ctx, dsClient, flagset.Arg(0))
	if err != nil {
		return err
	}

	fmt.Println(string(val))
	return nil
}

func cliAdminSet(ctx context.Context, flagset *flag.FlagSet, args []string) error {
	var (
		creds     = flagset.String("creds", "", "credentials file")
		projectID = flagset.String("project", "unclog", "project ID")
		test      = flagset.Bool("test", false, "run in test mode")
	)

	err := flagset.Parse(args)
	if err != nil {
		return err
	}

	if flagset.NArg() != 2 {
		return errors.New("usage: unclog admin set VAR VALUE")
	}

	if *test {
		if *creds != "" {
			return fmt.Errorf("cannot supply both -test and -creds")
		}

		err := aesite.DSTest(ctx, *projectID)
		if err != nil {
			return err
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

	return aesite.SetSetting(ctx, dsClient, flagset.Arg(0), []byte(flagset.Arg(1)))
}
