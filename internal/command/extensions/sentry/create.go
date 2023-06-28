package sentry_ext

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
)

func create() (cmd *cobra.Command) {

	const (
		short = "Provision a Sentry project for a Fly.io app"
		long  = short + "\n"
	)

	cmd = command.New("create", short, long, runSentryCreate, command.RequireSession, command.RequireAppName)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runSentryCreate(ctx context.Context) (err error) {

	_, err = extensions_core.ProvisionExtension(ctx, extensions_core.ExtensionOptions{
		Provider:     "sentry",
		SelectName:   false,
		SelectRegion: false,
	})

	return
}
