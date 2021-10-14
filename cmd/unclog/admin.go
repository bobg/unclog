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

	"github.com/bobg/unclog"
)

func (c maincmd) cliAdmin(ctx context.Context, args []string) error {
	return subcmd.Run(ctx, admincmd{maincmd: c}, args)
}

type admincmd struct {
	maincmd
}

func (c admincmd) Subcmds() subcmd.Map {
	return subcmd.Commands(
		"get", c.cliAdminGet, "get a parameter value", subcmd.Params(
			"param", subcmd.String, "", "parameter name to get",
		),
		"set", c.cliAdminSet, "set a parameter value", subcmd.Params(
			"param", subcmd.String, "", "parameter name to set",
			"val", subcmd.String, "", "parameter value",
		),
		"kick", c.cliAdminKick, "kick the service", subcmd.Params(
			"-date", subcmd.String, "", "process threads with this date (YYYY-MM-DD, default one week ago)",
			"addr", subcmd.String, "", "Gmail address",
		),
		"session", c.cliAdminSession, "show the details of a session", subcmd.Params(
			"cookie", subcmd.String, "", "session cookie",
		),
	)
}

func (c admincmd) cliAdminGet(ctx context.Context, param string, _ []string) error {
	dsClient, err := c.dsClient(ctx)
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

func (c admincmd) cliAdminSet(ctx context.Context, param, val string, _ []string) error {
	dsClient, err := c.dsClient(ctx)
	if err != nil {
		return errors.Wrap(err, "creating datastore client")
	}

	return aesite.SetSetting(ctx, dsClient, param, []byte(val))
}

func (c admincmd) cliAdminKick(ctx context.Context, date, addr string, _ []string) error {
	dsClient, err := c.dsClient(ctx)
	if err != nil {
		return errors.Wrap(err, "creating datastore client")
	}

	masterKey, err := aesite.GetSetting(ctx, dsClient, "master-key")
	if err != nil {
		return errors.Wrap(err, "getting master key")
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

	req, err := http.NewRequestWithContext(ctx, "POST", "https://unclog.appspot.com/push", bytes.NewReader(jsonMsg))
	if err != nil {
		return errors.Wrap(err, "creating POST request")
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Unclog-Key", string(masterKey))

	var httpClient http.Client
	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "posting to /push URL")
	}
	return resp.Body.Close()
}

func (c admincmd) cliAdminSession(ctx context.Context, cookie string, _ []string) error {
	dsClient, err := c.dsClient(ctx)
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
