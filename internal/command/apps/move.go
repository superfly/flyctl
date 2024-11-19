package apps

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command/deploy/statics"
	"github.com/superfly/flyctl/internal/flag/completion"
	"github.com/superfly/flyctl/internal/flyutil"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/logger"
	mach "github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newMove() *cobra.Command {
	const (
		long = `Move an application to another
organization the current user belongs to.
For details, see https://fly.io/docs/apps/move-app-org/.`
		short = "Move an app to another organization."
		usage = "move <app name>"
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
			Description: "Update machines without waiting for health checks",
			Default:     false,
		},
	)

	move.ValidArgsFunction = completion.Adapt(completion.CompleteApps)

	return move
}

// TODO: make internal once the move package is removed
func RunMove(ctx context.Context) error {
	var (
		appName  = flag.FirstArg(ctx)
		client   = flyutil.ClientFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		colorize = io.ColorScheme()
		logger   = logger.FromContext(ctx)
	)

	app, err := client.GetApp(ctx, appName)
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

	return runMoveAppOnMachines(ctx, app, org)
}

func runMoveAppOnMachines(ctx context.Context, app *fly.App, targetOrg *fly.Organization) error {
	var (
		client           = flyutil.ClientFromContext(ctx)
		io               = iostreams.FromContext(ctx)
		skipHealthChecks = flag.GetBool(ctx, "skip-health-checks")
	)

	ctx, err := BuildContext(ctx, app.Compact())
	if err != nil {
		return err
	}

	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx)
	defer releaseLeaseFunc()
	if err != nil {
		return err
	}

	oldOrg, err := client.GetOrganizationBySlug(ctx, app.Organization.Slug)
	if err != nil {
		return fmt.Errorf("failed to find app's original organization: %w", err)
	}

	oldStaticsBucket, err := statics.FindBucket(ctx, app, oldOrg)
	if err != nil {
		return fmt.Errorf("failed to find app's original statics bucket: %w", err)
	}

	if _, err := client.MoveApp(ctx, app.Name, targetOrg.ID); err != nil {
		return fmt.Errorf("failed moving app: %w", err)
	}

	if oldStaticsBucket != nil {
		err := statics.MoveBucket(ctx, oldStaticsBucket, oldOrg, app, targetOrg, machines)
		if err != nil {
			return fmt.Errorf("failed to move statics bucket: %w", err)
		}
	}

	for _, machine := range machines {
		input := &fly.LaunchMachineInput{
			Name:             machine.Name,
			Region:           machine.Region,
			Config:           machine.Config,
			SkipHealthChecks: skipHealthChecks,
		}
		mach.Update(ctx, machine, input)
	}
	fmt.Fprintf(io.Out, "successfully moved %s to %s\n", app.Name, targetOrg.Name)

	return nil
}
