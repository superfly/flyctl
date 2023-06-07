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
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/command/machine"
	"github.com/superfly/flyctl/internal/command/status"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

const (
	yellowBrickRoad = `
Oops! We ran into issues migrating your app.
We're constantly working to improve the migration and squash bugs, but for
now please let this debug wizard guide you down a yellow brick road
of potential solutions...
               ,,,,,                    
       ,,.,,,,,,,,, .                   
   .,,,,,,,                             
  ,,,,,,,,,.,,                          
     ,,,,,,,,,,,,,,,,,,,                
         ,,,,,,,,,,,,,,,,,,,,           
            ,,,,,,,,,,,,,,,,,,,,,       
           ,,,,,,,,,,,,,,,,,,,,,,,      
        ,,,,,,,,,,,,,,,,,,,,,,,,,,,,.   
   , ,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,,

`
)

func newTroubleshoot() *cobra.Command {
	const (
		usage = `troubleshoot`
		long  = `Troubleshoot an app that has been migrated to Apps V2`
		short = long
	)
	cmd := command.New(
		usage, short, long, runTroubleshoot,
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

func runTroubleshoot(ctx context.Context) (err error) {
	var (
		appName     = appconfig.NameFromContext(ctx)
		flapsClient *flaps.Client
	)

	defer func() {
		if err != nil {
			err = wrapTroubleshootingErrWithSuggestions(err)
		}
	}()

	flapsClient, err = flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return fmt.Errorf("could not create flaps client: %w", err)
	}

	ctx = flaps.NewContext(ctx, flapsClient)

	t, err := newTroubleshooter(ctx, appName)
	if err != nil {
		return err
	}

	return t.run(ctx)
}

func wrapTroubleshootingErrWithSuggestions(err error) error {
	return fmt.Errorf("%w%s", err, `
please try running 'fly migrate-to-v2 troubleshoot' later.
if the problem persists, try bringing it up in the community forum (https://community.fly.io),
or if you have one, your plan's support mailbox`)
}

type troubleshooter struct {
	app      *api.AppCompact
	machines []*api.Machine
	allocs   []*api.AllocationStatus
}

func newTroubleshooter(ctx context.Context, appName string) (*troubleshooter, error) {
	apiClient := client.FromContext(ctx).API()

	app, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, err
	}

	t := &troubleshooter{
		app: app,
	}
	if err := t.refreshMachines(ctx); err != nil {
		return nil, err
	}
	if !t.hasMigrated(ctx) {
		return nil, fmt.Errorf("app has not been migrated to Apps V2")
	}
	if err := t.refreshAllocs(ctx); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *troubleshooter) refreshMachines(ctx context.Context) error {

	flapsClient := flaps.FromContext(ctx)

	var err error
	if t.app.PlatformVersion != appconfig.NomadPlatform {
		t.machines, err = flapsClient.List(ctx, "")
		if err != nil {
			return fmt.Errorf("could not list machines: %w", err)
		}
	}
	return nil
}

func (t *troubleshooter) refreshAllocs(ctx context.Context) error {
	apiClient := client.FromContext(ctx).API()
	var err error

	t.allocs, err = apiClient.GetAllocations(ctx, t.app.Name, false)
	if err != nil {
		return fmt.Errorf("could not list Nomad VMs: %w", err)
	}
	return nil
}

func (t *troubleshooter) hasMigrated(ctx context.Context) bool {
	client := client.FromContext(ctx).API()

	if t.app.PlatformVersion == appconfig.DetachedPlatform {
		return true
	}

	// Check that the app is not currently on nomad
	if t.app.PlatformVersion == appconfig.NomadPlatform {
		return false
	}

	// Look for a machine tied to a previous alloc
	for _, machine := range t.machines {
		if machine.Config != nil && machine.Config.Metadata != nil {
			if _, ok := machine.Config.Metadata[api.MachineConfigMetadataKeyFlyPreviousAlloc]; ok {
				return true
			}
		}
	}

	// Look for a release created by admin-bot@fly.io
	releases, err := client.GetAppReleasesMachines(ctx, t.app.Name, "", 25)
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

func (t *troubleshooter) run(ctx context.Context) error {
	io := iostreams.FromContext(ctx)

	if t.app.PlatformVersion == appconfig.MachinesPlatform && len(t.allocs) == 0 {

		fmt.Fprintf(io.Out, "No issues detected.\n")
	}

	// From here on out we know that we're either
	//   * not on the machines platform, or
	//   * we have at least one nomad alloc
	// (meaning: we've got issues)

	fmt.Fprint(io.Out, yellowBrickRoad)

	if t.app.PlatformVersion != appconfig.MachinesPlatform {
		err := t.unsuspend(ctx)
		if err != nil {
			return fmt.Errorf("could not unsuspend app: %w", err)
		}
	}

	switch t.app.PlatformVersion {
	case appconfig.NomadPlatform:
		// Already handled in newTroubleshooter by the hasMigrated check
		return nil
	case appconfig.MachinesPlatform:
		fmt.Fprintf(io.Out, "Detected Nomad VMs running on V2 app, cleaning up.\n")
		return t.cleanupNomadSwitchToMachines(ctx)
	case appconfig.DetachedPlatform:
		fmt.Fprintf(io.Out, `The app's platform version is 'detached'
This means that the app is stuck in a half-migrated state, and wasn't able to
be fully recovered during the migration error rollback process.

Fixing this depends on how far the app got in the migration process.
Please use these tools to troubleshoot and attempt to repair the app.
`)
		return t.detachedInteractiveTroubleshoot(ctx)
	}

	return fmt.Errorf("unknown platform version: %s", t.app.PlatformVersion)
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

func (t *troubleshooter) unsuspend(ctx context.Context) error {

	if t.app.Status != "suspended" {
		return nil
	}

	client := client.FromContext(ctx).API()
	if t.app.Status == "suspended" {
		_, err := client.ResumeApp(ctx, t.app.Name)
		if err != nil {
			return err
		}
	}

	err := backoffWait(ctx, 1*time.Minute, func() (bool, error) {
		app, err := client.GetAppCompact(ctx, t.app.Name)
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

func (t *troubleshooter) cleanupNomadSwitchToMachines(ctx context.Context) error {

	io := iostreams.FromContext(ctx)

	if t.app.PlatformVersion != appconfig.DetachedPlatform {
		err := t.setPlatformVersion(ctx, appconfig.DetachedPlatform)
		if err != nil {
			return err
		}
	}

	fmt.Fprintf(io.Out, "Destroying Nomad VMs and setting platform version to machines/Apps V2.\n")
	vmGroups := lo.Uniq(lo.Map(t.allocs, func(alloc *api.AllocationStatus, _ int) string {
		return alloc.TaskName
	}))
	err := scaleNomadToZero(ctx, t.app, "", vmGroups)
	if err != nil {
		return err
	}
	err = t.setPlatformVersion(ctx, appconfig.MachinesPlatform)
	if err != nil {
		return err
	}
	fmt.Fprint(io.Out, "Done!\n")
	return nil
}

func (t *troubleshooter) setPlatformVersion(ctx context.Context, ver string) error {
	return apps.UpdateAppPlatformVersion(ctx, appconfig.NameFromContext(ctx), ver)
}

func (t *troubleshooter) detachedAutodiagnose(ctx context.Context) string {

	var (
		missingProcessGroup = false
		anyIssues           = false
		errorStr            = ""
	)

	errorStr += "Process group issues:\n"
	{
		nomadTaskGroups := make(map[string]bool)
		for _, alloc := range t.allocs {
			nomadTaskGroups[alloc.TaskName] = false
		}
		for _, machine := range t.machines {
			nomadTaskGroups[machine.ProcessGroup()] = true
		}
		for taskGroup, hasMachine := range nomadTaskGroups {
			if !hasMachine {
				errorStr += fmt.Sprintf(" * '%s' has no machines\n", taskGroup)
				missingProcessGroup = true
				anyIssues = true
			}
		}
		if !missingProcessGroup {
			errorStr += " * none found\n"
		}
	}

	errorStr += "\nVM count issues:\n"
	{
		if len(t.allocs) > len(t.machines) {
			errorStr += fmt.Sprintf(" * %d more Nomad VMs than machines\n", len(t.allocs)-len(t.machines))
			anyIssues = true
		} else {
			errorStr += " * none found\n"
		}
	}

	errorStr += "\nTo fix this, you can try:\n"
	if !anyIssues {
		errorStr += " * removing Nomad VMs and switching to V2\n"
	} else {
		if missingProcessGroup {
			errorStr += " * running `fly deploy` to create missing process groups,\n   then removing existing Nomad VMs and switching to V2\n"
		} else {
			errorStr += " * removing existing Nomad VMs and switching to V2, then using `fly scale count` to scale your app as needed\n"
		}
	}
	return errorStr
}

func (t *troubleshooter) detachedInteractiveTroubleshoot(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()

	if len(t.machines) == 0 && len(t.allocs) == 0 {
		fmt.Fprintf(io.Out, "No machines or Nomad VMs found. Setting platform version to machines/Apps V2.\n")
		return t.setPlatformVersion(ctx, appconfig.MachinesPlatform)
	}

	if len(t.machines) == 0 {
		fmt.Fprintf(io.Out, "No Apps v2 machines found. Setting platform version to Nomad.\n")
		t.setPlatformVersion(ctx, appconfig.NomadPlatform)
	}

	if len(t.allocs) == 0 {
		fmt.Fprintf(io.Out, "No legacy Nomad VMs found. Setting platform version to machines/Apps V2.\n")
		return t.setPlatformVersion(ctx, appconfig.MachinesPlatform)
	}

	if !io.IsInteractive() {
		fmt.Fprintf(io.Out, "Troubleshooting mode is intended for interactive use.\nOutput of autodiagnose:\n%s\n", t.detachedAutodiagnose(ctx))
		return nil
	}

	const (
		Autodiagnose            = "Autodiagnose issues"
		PrintNomad              = "List Nomad VMs"
		PrintMachines           = "List Machines"
		Deploy                  = "Deploy Machines (equivalent to 'fly deploy', might help with process groups)"
		DestroyNomadUseMachines = "Destroy remaining Nomad VMs and use Apps V2"
		DestroyMachinesUseNomad = "Destroy existing machines and use Nomad"
		Exit                    = "Exit"
	)

	// Lifted from command/status/status.go
	var appStatus *api.AppStatus
	var err error
	if appStatus, err = client.GetAppStatus(ctx, t.app.Name, false); err != nil {
		return fmt.Errorf("failed retrieving app %s: %w", t.app.Name, err)
	}
	var backupRegions []api.Region
	if appStatus.Deployed {
		if _, backupRegions, err = client.ListAppRegions(ctx, t.app.Name); err != nil {
			return fmt.Errorf("failed retrieving backup regions for %s: %w", t.app.Name, err)
		}
	}

	choice := Autodiagnose
	for {
		fmt.Fprintf(io.Out, "\n")
		err := survey.AskOne(&survey.Select{
			Message: "What would you like to do?",
			Options: []string{
				Autodiagnose,
				PrintNomad,
				PrintMachines,
				Deploy,
				DestroyNomadUseMachines,
				DestroyMachinesUseNomad,
				Exit,
			},
			Default: choice,
		}, &choice)
		if err != nil {
			return err
		}
		switch choice {
		case Autodiagnose:
			fmt.Fprint(io.Out, t.detachedAutodiagnose(ctx))
		case PrintNomad:
			fmt.Fprint(io.Out, "Nomad VMs:\n")
			err = render.AllocationStatuses(io.Out, "Nomad VMs", backupRegions, appStatus.Allocations...)
			if err != nil {
				return err
			}
		case PrintMachines:
			if err := status.RenderMachineStatus(ctx, t.app, io.Out); err != nil {
				return err
			}
		case Deploy:
			if err := t.deployMachines(ctx); err != nil {
				fmt.Fprintf(io.ErrOut, "Error deploying machines: %s\n", err)
			}
			err = t.refreshMachines(ctx)
			if err != nil {
				return err
			}
		case DestroyNomadUseMachines:
			confirm := false
			if err := survey.AskOne(&survey.Confirm{
				Message: "Are you sure you want to remove existing Nomad VMs and switch to V2?",
			}, &confirm); err != nil {
				return err
			}
			if !confirm {
				continue
			}

			return t.cleanupNomadSwitchToMachines(ctx)
		case DestroyMachinesUseNomad:
			confirm := false
			if err := survey.AskOne(&survey.Confirm{
				Message: "Are you sure you want to remove all Machines and switch back to Nomad?",
			}, &confirm); err != nil {
				return err
			}
			if !confirm {
				continue
			}

			fmt.Fprint(io.Out, "Destroying machines and setting platform version to nomad.\n")

			for _, mach := range t.machines {
				err := machine.Destroy(ctx, t.app, mach, true)
				if err != nil {
					return fmt.Errorf("could not destroy machine: %w", err)
				}
			}
			err = t.setPlatformVersion(ctx, appconfig.NomadPlatform)
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

func (t *troubleshooter) deployMachines(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	tb := render.NewTextBlock(ctx, "Verifying app config")

	var cfg *appconfig.Config
	if cfg = appconfig.ConfigFromContext(ctx); cfg == nil {
		cfg, err = appconfig.FromRemoteApp(ctx, t.app.Name)
		if err != nil {
			return err
		}
	}

	cfg.AppName = t.app.Name

	err, extraInfo := cfg.Validate(ctx)
	if extraInfo != "" {
		fmt.Fprintf(io.Out, extraInfo)
	}
	if err != nil {
		return err
	}

	tb.Done("Verified app config")

	// Hack!
	newDeployCmd := deploy.New()
	fakeFlagCtx := flag.NewContext(ctx, newDeployCmd.Flags())

	if err := flag.SetString(fakeFlagCtx, flag.AppConfigFilePathName, flag.GetAppConfigFilePath(ctx)); err != nil {
		return err
	}
	if err := flag.SetString(fakeFlagCtx, flag.AppName, flag.GetApp(ctx)); err != nil {
		return err
	}

	return deploy.DeployWithConfig(fakeFlagCtx, cfg, false)
}
