package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
)

var commands = map[string]func(context.Context, *flag.FlagSet, []string) error{
	"admin": cliAdmin,
	"serve": cliServe,
}

func main() {
	ctx := context.Background()

	flag.Parse()

	flagset := flag.NewFlagSet("", flag.ContinueOnError)

	if flag.NArg() == 0 {
		log.Print("running server in default mode")
		err := cliServe(ctx, flagset, nil)
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	cmd := flag.Arg(0)
	fn, ok := commands[cmd]
	if !ok {
		log.Fatalf("unknown command %s", cmd)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		sig := <-sigCh
		log.Printf("got signal %s", sig)
		cancel()
	}()

	args := flag.Args()
	err := fn(ctx, flagset, args[1:])
	if err != nil {
		log.Fatal(err)
	}
}
