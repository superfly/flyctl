package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/iostreams"
	"gopkg.in/yaml.v2"
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
		ctx = flaps.NewContext(ctx, flapsClient)

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

	var b []byte
	var err error

	if flag.GetBool(ctx, "yaml") {
		b, err = yaml.Marshal(cfg)
	} else if flag.GetBool(ctx, "toml") {
		b, err = toml.Marshal(cfg)
	} else {
		b, err = json.MarshalIndent(cfg, "", "  ")
	}

	if err != nil {
		return err
	}
	fmt.Fprintln(io.Out, string(b))
	return nil
}
