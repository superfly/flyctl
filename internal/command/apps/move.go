package apps

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/appsecrets"
	"github.com/superfly/flyctl/internal/command/deploy/statics"
	"github.com/superfly/flyctl/internal/flag/completion"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
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
For details, see https://docs.fly.io/apps/move-app-org`
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
		appName     = flag.FirstArg(ctx)
		flapsClient = flapsutil.ClientFromContext(ctx)
		io          = iostreams.FromContext(ctx)
		colorize    = io.ColorScheme()
		logger      = logger.FromContext(ctx)
	)

	app, err := flapsClient.GetApp(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed fetching app: %w", err)
	}

	logger.Infof("app %q is currently in organization %q", app.Name, app.Organization.Slug)
	org, err := prompt.Org(ctx)
	if err != nil {
		return err
	}

	// Flaps reports the raw org slug, while the org picked by the user may
	// carry the "personal" alias.
	if app.Organization.Slug == org.RawSlug || app.Organization.Slug == org.Slug {
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

func runMoveAppOnMachines(ctx context.Context, app *flaps.App, targetOrg *uiex.Organization) error {
	var (
		client           = flyutil.ClientFromContext(ctx)
		io               = iostreams.FromContext(ctx)
		skipHealthChecks = flag.GetBool(ctx, "skip-health-checks")
	)

	ctx, err := BuildContextForApp(ctx, app)
	if err != nil {
		return err
	}

	machines, releaseLeaseFunc, err := mach.AcquireAllLeases(ctx, app.Name)
	defer releaseLeaseFunc()
	if err != nil {
		return err
	}

	oldOrg, err := uiexutil.AppOrganization(ctx, app)
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
		releaseVersion := 0
		if release, err := uiexutil.LatestRelease(ctx, uiexutil.ClientFromContext(ctx), app.Name); err != nil {
			return movedButErr(app, targetOrg, fmt.Errorf("failed to find app's current release: %w", err))
		} else if release != nil {
			releaseVersion = release.Version
		}

		err := statics.MoveBucket(ctx, oldStaticsBucket, oldOrg, app, targetOrg, releaseVersion, machines)
		if err != nil {
			return movedButErr(app, targetOrg, fmt.Errorf("failed to move statics bucket: %w", err))
		}
	}

	minvers, err := appsecrets.GetMinvers(app.Name)
	if err != nil {
		return movedButErr(app, targetOrg, fmt.Errorf("failed to read the app's secrets version: %w", err))
	}

	if err := updateMovedMachines(ctx, app.Name, machines, skipHealthChecks, minvers); err != nil {
		return movedButErr(app, targetOrg, err)
	}
	fmt.Fprintf(io.Out, "successfully moved %s to %s\n", app.Name, targetOrg.Name)

	return nil
}

// updateMovedMachines updates every Machine, even after a failure: the app has
// already moved.
func updateMovedMachines(ctx context.Context, appName string, machines []*fly.Machine, skipHealthChecks bool, minvers *uint64) error {
	var (
		errs   []error
		failed int
	)
	for i, machine := range machines {
		// Stop on Ctrl-C rather than send updates that are bound to fail.
		if ctx.Err() != nil {
			errs = append(errs, fmt.Errorf("stopped before updating %d Machines: %w", len(machines)-i, context.Cause(ctx)))
			break
		}
		input := &fly.LaunchMachineInput{
			Name:              machine.Name,
			Region:            machine.Region,
			Config:            machine.Config,
			SkipHealthChecks:  skipHealthChecks,
			MinSecretsVersion: minvers,
		}
		if err := mach.Update(ctx, appName, machine, input); err != nil {
			// Most mach.Update errors already name the Machine.
			if !strings.Contains(err.Error(), machine.ID) {
				err = fmt.Errorf("machine %s: %w", machine.ID, err)
			}
			errs = append(errs, err)
			failed++
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to update %d of %d Machines:\n%w", failed, len(machines), errors.Join(errs...))
	}

	return nil
}

// movedButErr reports a failure after the app has already moved, so that the
// user doesn't retry the move.
func movedButErr(app *flaps.App, targetOrg *uiex.Organization, err error) error {
	return fmt.Errorf("moved %s to %s, but %w", app.Name, targetOrg.Name, err)
}
