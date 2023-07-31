package sentry_ext

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/command/secrets"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/scanner"
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
	appName := appconfig.NameFromContext(ctx)

	absDir, err := filepath.Abs(".")

	if err != nil {
		return err
	}

	srcInfo, err := scanner.Scan(absDir, &scanner.ScannerConfig{})

	if err != nil {
		return err
	}

	options := gql.AddOnOptions{}

	if srcInfo != nil && PlatformMap[srcInfo.Family] != "" {
		options["platform"] = PlatformMap[srcInfo.Family]
	}

	sentryOptions := extensions_core.MonitoringExtensionDefaults
	sentryOptions.Options = options

	extension, err := extensions_core.ProvisionExtension(ctx, appName, sentryOptions)
	secrets.DeploySecrets(ctx, gql.ToAppCompact(extension.App), false, false)
	return
}
