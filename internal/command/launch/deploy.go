package launch

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

// firstDeploy performs the first deploy of an app.
// Note that this function checks and respects the --no-deploy flag, so it may not actually deploy.
func (state *launchState) firstDeploy(ctx context.Context) error {
	// This function is only called if state.sourceInfo is not nil, so we don't need to do any checking.

	io := iostreams.FromContext(ctx)

	ctx = appconfig.WithName(ctx, state.Plan.AppName)
	ctx = appconfig.WithConfig(ctx, state.appConfig)

	// Notices from a launcher about its behavior that should always be displayed
	if state.sourceInfo.Notice != "" {
		fmt.Fprintln(io.Out, state.sourceInfo.Notice)
	}

	// TODO(Allison): Do we want to make the executive decision to just *always* deploy?
	// Feedback(Sam): scanners need the abiiity to abort the deploy if they detect a problem

	deployNow := true
	// deployNow := false
	// promptForDeploy := true

	if state.sourceInfo.SkipDeploy || flag.GetBool(ctx, "no-deploy") {
		deployNow = false
		// promptForDeploy = false
	}

	if flag.GetBool(ctx, "now") {
		deployNow = true
		// promptForDeploy = false
	}

	if flag.GetBool(ctx, "no-create") {
		deployNow = false
	}

	/*
		if promptForDeploy {
			confirm, err := prompt.Confirm(ctx, "Would you like to deploy now?")
			if confirm && err == nil {
				deployNow = true
			}

			// Reload and validate the app config in case the user edited it before confirming
			if deployNow {
				path := appConfig.ConfigFilePath()
				newCfg, err := appconfig.LoadConfig(path)
				if err != nil {
					return fmt.Errorf("failed to reload configuration file %state: %w", path, err)
				}

				if appConfig.ForMachines() {
					if err := newCfg.SetMachinesPlatform(); err != nil {
						return fmt.Errorf("failed to parse fly.toml, check the configuration format: %w", err)
					}
				}
				appConfig = newCfg
			}
		}
	*/

	err, extraInfo := state.appConfig.Validate(ctx)
	if extraInfo != "" {
		fmt.Fprint(io.ErrOut, extraInfo)
	}
	if err != nil {
		return fmt.Errorf("invalid configuration file: %w", err)
	}

	if deployNow {
		// We want to allow overriding HA, but otherwise we want to inherit HA from the plan.
		if !flag.IsSpecified(ctx, "ha") {
			// TODO(Ali): This is a hack to get launch v2 shipping. We probably want to pipe the HA flag through deploy properly.
			if err := flag.SetString(ctx, "ha", lo.Ternary(state.Plan.HighAvailability, "true", "false")); err != nil {
				return err
			}
		}
		return deploy.DeployWithConfig(ctx, state.appConfig, 0, flag.GetBool(ctx, "now"))
	}

	// Alternative deploy documentation if our standard deploy method is not correct
	if state.sourceInfo.DeployDocs != "" {
		fmt.Fprintln(io.Out, state.sourceInfo.DeployDocs)
	} else {
		fmt.Fprintln(io.Out, "Your app is ready! Deploy with `flyctl deploy`")
	}

	return nil
}
