package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newMove() *cobra.Command {
	const (
		long = `The APPS MOVE command will move an application to another
organization the current user belongs to.
`
		short = "Move an app to another organization"
		usage = "move [APPNAME]"
	)

	move := command.New(usage, short, long, RunMove,
		command.RequireSession,
	)

	move.Args = cobra.ExactArgs(1)

	flag.Add(move,
		flag.Yes(),
		flag.Org(),
		flag.Bool{
			Name:        "skip-health-checks",
			Description: "Update machines without waiting for health checks. (Machines only)",
			Default:     false,
		},
	)

	return move
}

// TODO: make internal once the move package is removed
func RunMove(ctx context.Context) error {
	var (
		appName  = flag.FirstArg(ctx)
		client   = client.FromContext(ctx).API()
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		logger   = logger.FromContext(ctx)
	)

	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed fetching app: %w", err)
	}

	logger.Infof("app %q is currently in organization %q", app.Name, app.Organization.Slug)
	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	if app.Organization.Slug == org.Slug {
		fmt.Fprintln(io.Out, "No changes to apply")
		return nil
	}

	if !flag.GetYes(ctx) {
		const msg = `Moving an app between organizations requires a complete shutdown and restart. This will result in some app downtime.
If the app relies on other services within the current organization, it may not come back up in a healthy manner.
Please confirm whether you wish to restart this app now.`
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Move app %s?", appName); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	// Run machine specific migration process.
	if app.PlatformVersion == "machines" {
		return runMoveAppOnMachines(ctx, app, org)
	}

	_, err = client.MoveApp(ctx, appName, org.ID)
	if err != nil {
		return fmt.Errorf("failed moving app: %w", err)
	}

	fmt.Fprintf(io.Out, "successfully moved %s to %s\n", appName, org.Slug)

	return nil
}

func runMoveAppOnMachines(ctx context.Context, app *api.AppCompact, targetOrg *api.Organization) error {
	var (
		client           = client.FromContext(ctx).API()
		io               = iostreams.FromContext(ctx)
		skipHealthChecks = flag.GetBool(ctx, "skip-health-checks")
	)

	ctx, err := BuildContext(ctx, app)
	if err != nil {
		return err
	}

	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc(ctx, machines)
	if err != nil {
		return err
	}

	updatedApp, err := client.MoveApp(ctx, app.Name, targetOrg.ID)
	if err != nil {
		return fmt.Errorf("failed moving app: %w", err)
	}

	for _, machine := range machines {
		config := machine.Config
		config.Network = &api.MachineNetwork{ID: updatedApp.NetworkID}

		input := &api.LaunchMachineInput{
			AppID:            app.ID,
			ID:               machine.ID,
			Name:             machine.Name,
			Region:           machine.Region,
			OrgSlug:          targetOrg.ID,
			Config:           config,
			SkipHealthChecks: skipHealthChecks,
		}
		mach.Update(ctx, machine, input)
	}
	fmt.Fprintf(io.Out, "successfully moved %s to %s\n", app.Name, targetOrg.Name)

	return nil
}
