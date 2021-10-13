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
	"github.com/bobg/subcmd/v2"
	"github.com/pkg/errors"
	"google.golang.org/api/option"

	"github.com/bobg/unclog"
)

func cliAdmin(ctx context.Context, args []string) error {
	return subcmd.Run(ctx, admincmd{}, args)
}

type admincmd struct{}

func (admincmd) Subcmds() subcmd.Map {
	return subcmd.Commands(
		"get", cliAdminGet, "get a parameter value", subcmd.Params(
			"-creds", subcmd.String, "", "credentials file",
			"-project", subcmd.String, "unclog", "project ID",
			"-test", subcmd.Bool, false, "run in test mode",
			"param", subcmd.String, "", "parameter name to get",
		),
		"set", cliAdminSet, "set a parameter value", subcmd.Params(
			"-creds", subcmd.String, "", "credentials file",
			"-project", subcmd.String, "unclog", "project ID",
			"-test", subcmd.Bool, false, "run in test mode",
			"param", subcmd.String, "", "parameter name to set",
			"val", subcmd.String, "", "parameter value",
		),
		"kick", cliAdminKick, "kick the service", subcmd.Params(
			"-date", subcmd.String, "", "process threads with this date (YYYY-MM-DD, default one week ago)",
			"addr", subcmd.String, "", "Gmail address",
		),
		"session", cliAdminSession, "show the details of a session", subcmd.Params(
			"-creds", subcmd.String, "", "credentials file",
			"-project", subcmd.String, "unclog", "project ID",
			"-test", subcmd.Bool, false, "run in test mode",
			"cookie", subcmd.String, "", "session cookie",
		),
	)
}

func cliAdminGet(ctx context.Context, creds, projectID string, test bool, param string, _ []string) error {
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

	val, err := aesite.GetSetting(ctx, dsClient, param)
	if err != nil {
		return err
	}

	fmt.Println(string(val))
	return nil
}

func cliAdminSet(ctx context.Context, creds, projectID string, test bool, param, val string, _ []string) error {
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

	return aesite.SetSetting(ctx, dsClient, param, []byte(val))
}

func cliAdminKick(ctx context.Context, date, addr string, _ []string) error {
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

func cliAdminSession(ctx context.Context, creds, projectID string, test bool, cookie string, _ []string) error {
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

	key, err := datastore.DecodeKey(cookie)
	if err != nil {
		return errors.Wrap(err, "decoding cookie")
	}

	sess, err := aesite.GetSessionByKey(ctx, dsClient, key)
	if err != nil {
		return errors.Wrap(err, "getting session")
	}

	fmt.Printf("%+v\n", *sess)

	return nil
}
