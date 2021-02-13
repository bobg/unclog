package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"cloud.google.com/go/datastore"
	"github.com/bobg/aesite"
	"github.com/bobg/subcmd"
	"github.com/pkg/errors"
	"google.golang.org/api/option"

	"github.com/bobg/unclog"
)

func cliAdmin(ctx context.Context, args []string) error {
	return subcmd.Run(ctx, admincmd{}, args)
}

type admincmd struct{}

func (admincmd) Subcmds() map[string]subcmd.Subcmd {
	return subcmd.Commands(
		"get", cliAdminGet, subcmd.Params(
			"creds", subcmd.String, "", "credentials file",
			"project", subcmd.String, "unclog", "project ID",
			"test", subcmd.Bool, false, "run in test mode",
		),
		"set", cliAdminSet, subcmd.Params(
			"creds", subcmd.String, "", "credentials file",
			"project", subcmd.String, "unclog", "project ID",
			"test", subcmd.Bool, false, "run in test mode",
		),
		"kick", cliAdminKick, subcmd.Params(
			"addr", subcmd.String, "", "Gmail address (required)",
			"date", subcmd.String, "", "process threads with this date (YYYY-MM-DD, optional)",
		),
	)
}

func cliAdminGet(ctx context.Context, creds, projectID string, test bool, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: unclog admin get VAR")
	}

	if test {
		if creds != "" {
			return fmt.Errorf("cannot supply both -test and -creds")
		}

		err := aesite.DSTest(ctx, projectID)
		if err != nil {
			return err
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

	val, err := aesite.GetSetting(ctx, dsClient, args[0])
	if err != nil {
		return err
	}

	fmt.Println(string(val))
	return nil
}

func cliAdminSet(ctx context.Context, creds, projectID string, test bool, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: unclog admin set VAR VALUE")
	}

	if test {
		if creds != "" {
			return fmt.Errorf("cannot supply both -test and -creds")
		}

		err := aesite.DSTest(ctx, projectID)
		if err != nil {
			return err
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

	return aesite.SetSetting(ctx, dsClient, args[0], []byte(args[1]))
}

func cliAdminKick(ctx context.Context, addr, date string, args []string) error {
	if addr == "" {
		return errors.New("-addr is required")
	}

	payload := unclog.PushPayload{Addr: addr}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return errors.Wrap(err, "JSON-marshaling payload")
	}
	base64Payload := base64.StdEncoding.EncodeToString(jsonPayload)

	msg := unclog.PushMessage{Date: date}
	msg.Message.Data = base64Payload

	jsonMsg, err := json.Marshal(msg)
	if err != nil {
		return errors.Wrap(err, "JSON-marshaling message")
	}

	resp, err := http.Post("https://unclog.appspot.com/push", "application/json", bytes.NewReader(jsonMsg))
	if err != nil {
		return errors.Wrap(err, "posting to /push URL")
	}
	resp.Body.Close()

	return nil
}
