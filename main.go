package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cli"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func main() {
	os.Exit(run())
}

func run() (exitCode int) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	if !buildinfo.IsDev() {
		defer func() {
			if sentry.Recover() {
				exitCode = 2
			}
		}()
	}

	exitCode = cli.Run(ctx, iostreams.System(), os.Args[1:]...)

	return
}
