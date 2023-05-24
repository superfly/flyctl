package config

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
)

func newSave() (cmd *cobra.Command) {
	const (
		short = "Save an app's config file"
		long  = `Save an application's configuration locally. The configuration data is
retrieved from the Fly service and saved in TOML format.`
	)
	cmd = command.New("save", short, long, runSave,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Yes(),
	)
	return
}

func runSave(ctx context.Context) error {
	var (
		err         error
		appName     = appconfig.NameFromContext(ctx)
		autoConfirm = flag.GetBool(ctx, "yes")
	)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	cfg, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}

	path := state.WorkingDirectory(ctx)
	if flag.IsSpecified(ctx, "config") {
		path = flag.GetString(ctx, "config")
	}
	configfilename, err := appconfig.ResolveConfigFileFromPath(path)
	if err != nil {
		return err
	}

	if exists, _ := appconfig.ConfigFileExistsAtPath(configfilename); exists && !autoConfirm {
		confirmation, err := prompt.Confirmf(ctx,
			"An existing configuration file has been found\nOverwrite file '%s'", configfilename)
		if err != nil {
			return err
		}
		if !confirmation {
			return nil
		}
	}

	return cfg.WriteToDisk(ctx, configfilename)
}
