package sentry_ext

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/secrets"
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
		extensions_core.SharedFlags,
	)
	return cmd
}

func runSentryCreate(ctx context.Context) (err error) {
	appName := appconfig.NameFromContext(ctx)

	extension, err := extensions_core.ProvisionExtension(ctx, extensions_core.ExtensionParams{
		AppName:  appName,
		Provider: "sentry",
	})
	if extension.SetsSecrets {
		err = secrets.DeploySecrets(ctx, gql.ToAppCompact(*extension.App), false, false)
	}
	return
}
