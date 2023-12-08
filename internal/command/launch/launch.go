package launch

import (
	"context"
	"fmt"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

// Launch launches the app described by the plan. This is the main entry point for launching a plan.
func (state *launchState) Launch(ctx context.Context) error {

	io := iostreams.FromContext(ctx)

	// TODO(Allison): are we still supporting the launch-into usecase?
	// I'm assuming *not* for now, because it's confusing UX and this
	// is the perfect time to remove it.

	state.updateConfig(ctx)

	app, err := state.createApp(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Created app '%s' in organization '%s'\n", app.Name, app.Organization.Slug)
	fmt.Fprintf(io.Out, "Admin URL: https://fly.io/apps/%s\n", app.Name)
	fmt.Fprintf(io.Out, "Hostname: %s.fly.dev\n", app.Name)

	if err = state.satisfyScannerBeforeDb(ctx); err != nil {
		return err
	}
	// TODO: Return rich info about provisioned DBs, including things
	//       like public URLs.
	err = state.createDatabases(ctx)
	if err != nil {
		return err
	}
	if err = state.satisfyScannerAfterDb(ctx); err != nil {
		return err
	}
	if err = state.createDockerIgnore(ctx); err != nil {
		return err
	}

	// Override internal port if requested using --internal-port flag
	if n := flag.GetInt(ctx, "internal-port"); n > 0 {
		state.appConfig.SetInternalPort(n)
	}

	// Finally write application configuration to fly.toml
	state.appConfig.SetConfigFilePath(state.configPath)
	if err := state.appConfig.WriteToDisk(ctx, state.configPath); err != nil {
		return err
	}

	if state.sourceInfo != nil {
		if err := state.firstDeploy(ctx); err != nil {
			return err
		}
	}

	return nil
}

// updateConfig populates the appConfig with the plan's values
func (state *launchState) updateConfig(ctx context.Context) {
	state.appConfig.AppName = state.Plan.AppName
	state.appConfig.PrimaryRegion = state.Plan.RegionCode
	if state.env != nil {
		state.appConfig.SetEnvVariables(state.env)
	}
	if state.Plan.HttpServicePort != 0 {
		if state.appConfig.HTTPService == nil {
			state.appConfig.HTTPService = &appconfig.HTTPService{
				ForceHTTPS:         true,
				AutoStartMachines:  api.Pointer(true),
				AutoStopMachines:   api.Pointer(true),
				MinMachinesRunning: api.Pointer(0),
				Processes:          []string{"app"},
			}
		}
		state.appConfig.HTTPService.InternalPort = state.Plan.HttpServicePort
	} else {
		state.appConfig.HTTPService = nil
	}
	state.appConfig.Compute = []*appconfig.Compute{
		{
			MachineGuest: state.Plan.Guest(),
			Processes:    lo.Keys(state.appConfig.Processes),
		},
	}
}

// createApp creates the fly.io app for the plan
func (state *launchState) createApp(ctx context.Context) (*api.App, error) {
	apiClient := client.FromContext(ctx).API()
	org, err := state.Org(ctx)
	if err != nil {
		return nil, err
	}

	f, err := flaps.NewFromAppName(ctx, state.Plan.AppName)
	if err != nil {
		return nil, err
	}

	if err := f.CreateApp(ctx, state.Plan.AppName, org.RawSlug); err != nil {
		return nil, err
	}

	return apiClient.GetApp(ctx, state.Plan.AppName)
}
