package config

import (
	"context"

	"github.com/spf13/cobra"
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
	flag.Add(cmd, flag.App(), flag.AppConfig())
	return
}

func runSave(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	cfg, err := appconfig.FromRemoteApp(ctx, appName)
	if err != nil {
		return err
	}

	cwd := state.WorkingDirectory(ctx)
	configfilename, err := appconfig.ResolveConfigFileFromPath(cwd)
	if err != nil {
		return err
	}

	if exists, _ := appconfig.ConfigFileExistsAtPath(configfilename); exists {
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
