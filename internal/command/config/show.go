package config

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newShow() (cmd *cobra.Command) {
	const (
		short = "Show an app's configuration"
		long  = `Print the app's configuration as JSON, or as YAML or TOML with --yaml or
--toml. By default it shows the configuration of the app's current
deployment; --local parses and shows the local fly.toml instead. To write
the deployed configuration to a file, use ` + "`fly config save`" + `.`
	)
	cmd = command.New("show", short, long, runShow,
		command.RequireSession,
		command.RequireAppName,
		command.LoadAppConfigIfPresent,
	)
	cmd.Args = cobra.NoArgs
	cmd.Aliases = []string{"display"}
	flag.Add(cmd, flag.App(), flag.AppConfig(),
		flag.Bool{
			Name:        "local",
			Description: "Parse and show local fly.toml file instead of fetching from the Fly service",
		},
		flag.Bool{
			Name:        "yaml",
			Description: "Show configuration in YAML format",
		},
		flag.Bool{
			Name:        "toml",
			Description: "Show configuration in TOML format",
		},
	)

	return
}

func runShow(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	var cfg *appconfig.Config

	if !flag.GetBool(ctx, "local") {
		var err error
		cfg, err = appconfig.FromRemoteApp(ctx, appName)
		if err != nil {
			return err
		}
	} else {
		cfg = appconfig.ConfigFromContext(ctx)
		if cfg == nil {
			return fmt.Errorf("No local fly.toml found")
		}
	}

	format := "json"

	if flag.GetBool(ctx, "yaml") {
		format = "yaml"
	} else if flag.GetBool(ctx, "toml") {
		format = "toml"
	}

	_, err := cfg.WriteTo(io.Out, format)

	if err != nil {
		return err
	}

	return nil
}
