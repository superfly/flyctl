package migrate_to_v2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/jpillora/backoff"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/apps"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/command/status"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newDebug() *cobra.Command {
	const (
		usage = `debug`
		long  = `Debug an app that has been migrated to Apps V2`
		short = long
	)
	cmd := command.New(
		usage, short, long, runDebug,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func hasMigrated(ctx context.Context, app *api.AppCompact, machines []*api.Machine) bool {
	client := client.FromContext(ctx).API()

	if app.PlatformVersion == appconfig.DetachedPlatform {
		return true
	}

	// Check that the app is not currently on nomad
	if app.PlatformVersion == appconfig.NomadPlatform {
		return false
	}

	// Look for a machine tied to a previous alloc
	for _, machine := range machines {
		if machine.Config != nil && machine.Config.Metadata != nil {
			if _, ok := machine.Config.Metadata[api.MachineConfigMetadataKeyFlyPreviousAlloc]; ok {
				return true
			}
		}
	}

	// Look for a release created by admin-bot@fly.io
	releases, err := client.GetAppReleasesMachines(ctx, app.Name, "", 25)
	if err != nil {
		return false
	}
	for _, release := range releases {
		// Technically, I don't think this is the only time we could use admin-bot@fly.io,
		// but we use it infrequently and soon we'll be done dealing with this,
		// so it's probably an acceptable way to determine this for now.
		if release.User.Email == "admin-bot@fly.io" {
			return true
		}
	}
	return false
}

const timedOutErr = "timed out"

func backoffWait(ctx context.Context, cutoff time.Duration, cb func() (bool, error)) error {
	ctx, cancelFn := context.WithTimeout(ctx, cutoff)
	defer cancelFn()
	b := &backoff.Backoff{
		Min:    1 * time.Second,
		Max:    10 * time.Second,
		Factor: 1.2,
		Jitter: true,
	}
	for {
		// Check for deadline
		select {
		case <-ctx.Done():
			return errors.New(timedOutErr)
		default:
		}
		done, err := cb()
		if err != nil {
			return err
		}
		if done {
			return nil
		}
		time.Sleep(b.Duration())
	}
}

func unsuspend(ctx context.Context, app *api.AppCompact) error {

	if app.Status != "suspended" {
		return nil
	}

	client := client.FromContext(ctx).API()
	if app.Status == "suspended" {
		_, err := client.ResumeApp(ctx, app.Name)
		if err != nil {
			return err
		}
	}

	err := backoffWait(ctx, 1*time.Minute, func() (bool, error) {
		app, err := client.GetAppCompact(ctx, app.Name)
		if err != nil {
			return false, err
		}
		if app.Status != "suspended" {
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		if err.Error() == timedOutErr {
			return errors.New("timed out waiting for app to unsuspend")
		}
		return err
	}

	return nil
}

func runDebug(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)
	client := client.FromContext(ctx).API()
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}
	flapsClient, err := flaps.NewFromAppName(ctx, app.Name)
	if err != nil {
		return fmt.Errorf("could not create flaps client: %w", err)
	}
	ctx = flaps.NewContext(ctx, flapsClient)

	// Grab the list of machines
	// Useful to have, but also used to determine if the app has been migrated
	var machines []*api.Machine
	if app.PlatformVersion != appconfig.NomadPlatform {
		machines, err = flapsClient.List(ctx, "")
		if err != nil {
			return fmt.Errorf("could not list machines: %w", err)
		}
	}

	if !hasMigrated(ctx, app, machines) {
		return fmt.Errorf("app has not been migrated to Apps V2")
	}

	if app.PlatformVersion != appconfig.MachinesPlatform {
		err := unsuspend(ctx, app)
		if err != nil {
			return fmt.Errorf("could not unsuspend app: %w", err)
		}
	}

	// Grab nomad allocs now that we know the app has been migrated
	allocs, err := client.GetAllocations(ctx, app.Name, false)
	if err != nil {
		return fmt.Errorf("could not list nomad allocs: %w", err)
	}

	if app.PlatformVersion == appconfig.DetachedPlatform {
		fmt.Fprintf(io.Out, `The app's platform version is 'detached'
This means that the app is stuck in a half-migrated state, and wasn't able to
be fully recovered during the migration error rollback process.

Fixing this depends on how far the app got in the migration process.
Please use these tools to troubleshoot and attempt to repair the app.
`)
		return fixDetachedApp(ctx, app, machines, allocs)
	}

	if app.PlatformVersion == appconfig.MachinesPlatform {
		if len(allocs) != 0 {
			fmt.Fprintf(io.Out, "Detected nomad allocs running on V2 app, cleaning up.\n")
			return zeroNomadUseMachines(ctx, app, allocs)
		}
	}

	fmt.Fprintf(io.Out, "No issues detected.\n")

	return nil
}

func zeroNomadUseMachines(ctx context.Context, app *api.AppCompact, allocs []*api.AllocationStatus) error {

	io := iostreams.FromContext(ctx)

	if app.PlatformVersion != appconfig.DetachedPlatform {
		err := setPlatformVersion(ctx, appconfig.DetachedPlatform)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(io.Out, "Destroying nomad allocs and setting platform version to machines/Apps V2.\n")
	vmGroups := lo.Uniq(lo.Map(allocs, func(alloc *api.AllocationStatus, _ int) string {
		return alloc.TaskName
	}))
	err := scaleNomadToZero(ctx, app, "", vmGroups)
	if err != nil {
		return err
	}
	err = setPlatformVersion(ctx, appconfig.MachinesPlatform)
	if err != nil {
		return err
	}
	fmt.Fprint(io.Out, "Done!\n")
	return nil
}

func setPlatformVersion(ctx context.Context, ver string) error {
	return apps.UpdateAppPlatformVersion(ctx, appconfig.NameFromContext(ctx), ver)
}

func fixDetachedApp(
	ctx context.Context,
	app *api.AppCompact,
	machines []*api.Machine,
	allocs []*api.AllocationStatus,
) error {
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()
	if len(machines) == 0 && len(allocs) == 0 {
		fmt.Fprintf(io.Out, "No machines or allocs found. Setting platform version to machines/Apps V2.\n")
		return setPlatformVersion(ctx, appconfig.MachinesPlatform)
	}

	if len(machines) == 0 {
		fmt.Fprintf(io.Out, "No Apps v2 machines found. Setting platform version to nomad.\n")
		setPlatformVersion(ctx, appconfig.NomadPlatform)
	}

	if len(allocs) == 0 {
		fmt.Fprintf(io.Out, "No legacy nomad allocs found. Setting platform version to machines/Apps V2.\n")
		return setPlatformVersion(ctx, appconfig.MachinesPlatform)
	}

	const (
		PrintNomad              = "List Nomad allocs"
		PrintMachines           = "List Machines"
		DestroyNomadUseMachines = "Destroy remaining Nomad allocs and use Apps V2"
		DestroyMachinesUseNomad = "Destroy existing machines and use Nomad"
		Exit                    = "Exit"
	)

	// Lifted from command/status/status.go
	var appStatus *api.AppStatus
	var err error
	if appStatus, err = client.GetAppStatus(ctx, app.Name, false); err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", app.Name, err)
	}
	var backupRegions []api.Region
	if appStatus.Deployed {
		if _, backupRegions, err = client.ListAppRegions(ctx, app.Name); err != nil {
			return fmt.Errorf("failed retrieving backup regions for %s: %w", app.Name, err)
		}
	}

	for {
		var opt struct{ Choice string }
		err := survey.Ask([]*survey.Question{{
			Name: "choice",
			Prompt: &survey.Select{
				Message: "What would you like to do?",
				Options: []string{
					PrintNomad,
					PrintMachines,
					DestroyNomadUseMachines,
					DestroyMachinesUseNomad,
					Exit,
				},
				Default: PrintMachines,
			},
		}}, &opt)
		if err != nil {
			return err
		}
		switch opt.Choice {
		case PrintNomad:
			fmt.Fprintf(io.Out, "Nomad allocs:\n")
			err = render.AllocationStatuses(io.Out, "Nomad Allocs", backupRegions, appStatus.Allocations...)
			if err != nil {
				return err
			}
		case PrintMachines:
			if err := status.RenderMachineStatus(ctx, app, io.Out); err != nil {
				return err
			}
		case DestroyNomadUseMachines:
			return zeroNomadUseMachines(ctx, app, allocs)
		case DestroyMachinesUseNomad:
			fmt.Fprintf(io.Out, "Destroying machines and setting platform version to nomad.\n")

			for _, mach := range machines {
				err := machine.Destroy(ctx, app, mach, true)
				if err != nil {
					return fmt.Errorf("could not destroy machine: %w", err)
				}
			}
			err = setPlatformVersion(ctx, appconfig.NomadPlatform)
			if err != nil {
				return err
			}
			fmt.Fprint(io.Out, "Done!\n")
			return nil
		case Exit:
			return nil
		}
	}
}
