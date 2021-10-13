package main

import (
	"context"
	"log"
	"os"

	"github.com/bobg/subcmd/v2"
)

func main() {
	err := subcmd.Run(context.Background(), maincmd{}, os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
}

type maincmd struct{}

func (maincmd) Subcmds() subcmd.Map {
	return subcmd.Commands(
		"admin", cliAdmin, "perform admin tasks", nil,
		"serve", cliServe, "run a server", subcmd.Params(
			"-creds", subcmd.String, "", "credentials file",
			"-project", subcmd.String, "unclog", "project ID",
			"-location", subcmd.String, "us-west2", "location ID",
			"-dir", subcmd.String, "web/build", "content dir",
			"-test", subcmd.Bool, false, "run in test mode",
		),
	)
}
