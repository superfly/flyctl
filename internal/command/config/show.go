package config

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
)

func newShow() (cmd *cobra.Command) {
	const (
		short = "Show an app's configuration"
		long  = `Show an application's configuration. The configuration is presented by default
in JSON format. The configuration data is retrieved from the Fly service.`
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
		flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
			AppName: appName,
		})
		if err != nil {
			return err
		}
		ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

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
