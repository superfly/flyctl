package launch

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
)

// Launch launches the app described by the plan. This is the main entry point for launching a plan.
func (state *launchState) Launch(ctx context.Context) error {
	ctx, span := tracing.GetTracer().Start(ctx, "state.launch")
	defer span.End()

	io := iostreams.FromContext(ctx)

	// TODO(Allison): are we still supporting the launch-into usecase?
	// I'm assuming *not* for now, because it's confusing UX and this
	// is the perfect time to remove it.

	if err := state.updateComputeFromDeprecatedGuestFields(ctx); err != nil {
		return err
	}

	state.updateConfig(ctx)

	if err := state.validateExtensions(ctx); err != nil {
		return err
	}

	org, err := state.Org(ctx)
	if err != nil {
		return err
	}
	if !planValidateHighAvailability(ctx, state.Plan, org, !state.warnedNoCcHa) {
		state.Plan.HighAvailability = false
		state.warnedNoCcHa = true
	}

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

	// Sentry
	if err = state.launchSentry(ctx, app.Name); err != nil {
		return err
	}

	// Finally write application configuration to fly.toml
	configDir := filepath.Dir(state.configPath)
	configPath := filepath.Join(configDir, flag.GetString(ctx, "config-file"))
	if flag.GetBool(ctx, "json") {
		configPath = strings.TrimSuffix(configPath, filepath.Ext(configPath)) + ".json"
	} else if flag.GetBool(ctx, "yaml") {
		configPath = strings.TrimSuffix(configPath, filepath.Ext(configPath)) + ".yaml"
	}
	state.appConfig.SetConfigFilePath(configPath)
	if err := state.appConfig.WriteToDisk(ctx, configPath); err != nil {
		return err
	}

	if state.sourceInfo != nil {
		if err := state.firstDeploy(ctx); err != nil {
			return err
		}
	}

	return nil
}

// Apply the freestanding Guest fields to the appConfig's Compute field
// This is temporary, but allows us to start using Compute-based plans in flyctl *now* while the UI catches up in time.
func (state *launchState) updateComputeFromDeprecatedGuestFields(ctx context.Context) error {
	if len(state.Plan.Compute) != 0 {
		// If the UI returns a compute field, then we don't need to do any forward-compat patching.
		return nil
	}
	// Fallback for versions of the UI that don't support a Compute field in the Plan.

	defer func() {
		// Set the plan's compute field to the calculated compute field.
		// This makes sure that code expecting a compute definition in the plan is able to find it
		// (and that it's up-to-date)
		state.Plan.Compute = state.appConfig.Compute
	}()

	if compute := state.appConfig.ComputeForGroup(state.appConfig.DefaultProcessName()); compute != nil {
		applyGuestToCompute(compute, state.Plan.Guest())
	} else {
		state.appConfig.Compute = append(state.appConfig.Compute, guestToCompute(state.Plan.Guest()))
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
				AutoStartMachines:  fly.Pointer(true),
				AutoStopMachines:   fly.Pointer(true),
				MinMachinesRunning: fly.Pointer(0),
				Processes:          []string{"app"},
			}
		}
		state.appConfig.HTTPService.InternalPort = state.Plan.HttpServicePort
	} else {
		state.appConfig.HTTPService = nil
	}
	state.appConfig.Compute = state.Plan.Compute
}

// createApp creates the fly.io app for the plan
func (state *launchState) createApp(ctx context.Context) (*fly.App, error) {
	apiClient := flyutil.ClientFromContext(ctx)
	org, err := state.Org(ctx)
	if err != nil {
		return nil, err
	}
	app, err := apiClient.CreateApp(ctx, fly.CreateAppInput{
		OrganizationID:  org.ID,
		Name:            state.Plan.AppName,
		PreferredRegion: &state.Plan.RegionCode,
		Machines:        true,
	})
	if err != nil {
		return nil, err
	}

	f, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{AppName: app.Name})
	if err != nil {
		return nil, err
	} else if err := f.WaitForApp(ctx, app.Name); err != nil {
		return nil, err
	}

	return app, nil
}
