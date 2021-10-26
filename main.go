package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/superfly/flyctl/cmd"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/internal/update"
)

func main() {
	defer func() {
		if sentry.Recover() {
			os.Exit(1)
		}
	}()

	flyctl.InitConfig()

	if err := run(); err != nil {
		flyerr.PrintCLIOutput(err)

		flyctl.BackgroundTaskWG.Wait()

		os.Exit(1)
	}
}

func run() error {
	client := client.New()
	if !client.IO.ColorEnabled() {
		// TODO: disable colors
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Kill, os.Interrupt)
	defer cancel()

	update.PromptFor(ctx)

	root := cmd.NewRootCmd(client)
	_, err := root.ExecuteContextC(ctx)
	return err
}
