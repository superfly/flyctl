package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
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

	err = keepPrevSections(ctx, cfg, configfilename)
	if err != nil {
		return err
	}

	return cfg.WriteToDisk(ctx, configfilename)
}

func keepPrevSections(ctx context.Context, currentCfg *appconfig.Config, configPath string) error {

	io := iostreams.FromContext(ctx)

	oldCfg, err := loadPrevConfig(configPath)
	if err != nil {
		return err
	}

	// Check if there's anything to actually copy over
	if oldCfg.Build == nil {
		return nil
	}

	if !flag.GetYes(ctx) {
		confirm := false
		fmt.Fprintf(io.Out, "\nSome sections of the config file are not kept remotely, such as the [build] section.\n")
		prompt := &survey.Confirm{
			Message: "Would you like to transfer the [build] section from the current config to the new one?",
		}
		err := survey.AskOne(prompt, &confirm)
		if err != nil {
			return err
		}
		if !confirm {
			return nil
		}
	}

	// Inherit the [build] section from the old config.
	// This is the only section from definition.SanitizedDefinition()
	// that is supported by the nomad platform and not synthesized.
	currentCfg.Build = oldCfg.Build

	return nil
}

func loadPrevConfig(configPath string) (*appconfig.Config, error) {
	cfg, err := appconfig.LoadConfig(configPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("error loading prev config: %w", err)
	}
	return cfg, nil
}
