package launch

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/docker/go-units"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
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

	planStep := plan.GetPlanStep(ctx)

	if !flag.GetBool(ctx, "no-create") && (planStep == "" || planStep == "create") {
		app, err := state.createApp(ctx)
		if err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "Created app '%s' in organization '%s'\n", app.Name, app.Organization.Slug)
		fmt.Fprintf(io.Out, "Admin URL: https://fly.io/apps/%s\n", app.Name)
		fmt.Fprintf(io.Out, "Hostname: %s.fly.dev\n", app.Name)

		if planStep == "create" {
			return nil
		}
	}

	// TODO: ideally this would be passed as a part of the plan to the Launch UI
	// and allow choices of what actions are desired to be make there.
	if state.sourceInfo != nil && state.sourceInfo.GitHubActions.Deploy {
		if planStep == "" || planStep == "generate" {
			if err = state.setupGitHubActions(ctx, state.Plan.AppName); err != nil {
				return err
			}
		}
	}

	if err = state.satisfyScannerBeforeDb(ctx); err != nil {
		return err
	}
	// TODO: Return rich info about provisioned DBs, including things
	//       like public URLs.

	if !flag.GetBool(ctx, "no-create") && planStep != "generate" {
		if err = state.createDatabases(ctx); err != nil {
			return err
		}
	}

	if planStep != "" && planStep != "deploy" && planStep != "generate" {
		return nil
	}

	if planStep == "" || planStep == "generate" {
		if err = state.satisfyScannerAfterDb(ctx); err != nil {
			return err
		}
		if err = state.createDockerIgnore(ctx); err != nil {
			return err
		}
	}

	// Sentry
	if !flag.GetBool(ctx, "no-create") {
		if err = state.launchSentry(ctx, state.Plan.AppName); err != nil {
			return err
		}
	}

	// if the user specified a command, set it in the app config
	if flag.GetString(ctx, "command") != "" {
		if state.appConfig.Processes == nil {
			state.appConfig.Processes = make(map[string]string)
		}

		state.appConfig.Processes["app"] = flag.GetString(ctx, "command")
	}

	volumes := flag.GetStringSlice(ctx, "volume")
	if len(volumes) > 0 {
		v := volumes[0]
		splittedIDDestOpts := strings.Split(v, ":")

		if len(splittedIDDestOpts) < 2 {
			re := regexp.MustCompile(`(?m)^VOLUME\s+(\[\s*")?(\/[\w\/]*?(\w+))("\s*\])?\s*$`)
			m := re.FindStringSubmatch(splittedIDDestOpts[0])

			if len(m) > 0 {
				state.appConfig.Mounts = []appconfig.Mount{
					{
						Source:      m[3], // last part of path
						Destination: m[2], // full path
					},
				}
			}
		} else {
			// if the user specified a volume, set it in the app config
			state.appConfig.Mounts = []appconfig.Mount{
				{
					Source:      splittedIDDestOpts[0],
					Destination: splittedIDDestOpts[1],
				},
			}

			if len(splittedIDDestOpts) > 2 {
				if err := ParseMountOptions(&state.appConfig.Mounts[0], splittedIDDestOpts[2]); err != nil {
					return err
				}
			}
		}
	}

	// Finally write application configuration to fly.toml
	configDir, configFile := filepath.Split(state.configPath)
	configFileOverride := flag.GetString(ctx, flagnames.AppConfigFilePath)
	if configFileOverride != "" {
		configFile = configFileOverride
	}

	// Resolve config format flags if applicable
	if flag.GetBool(ctx, "json") {
		configFile = strings.TrimSuffix(configFile, filepath.Ext(configFile)) + ".json"
	} else if flag.GetBool(ctx, "yaml") {
		configFile = strings.TrimSuffix(configFile, filepath.Ext(configFile)) + ".yaml"
	}

	configPath := filepath.Join(configDir, configFile)
	state.appConfig.SetConfigFilePath(configPath)
	if err := state.appConfig.WriteToDisk(ctx, configPath); err != nil {
		return err
	}

	if state.sourceInfo != nil {
		if state.appConfig.Deploy != nil && state.appConfig.Deploy.SeedCommand != "" {
			ctx = appconfig.WithSeedCommand(ctx, state.appConfig.Deploy.SeedCommand)
		}

		if err := state.firstDeploy(ctx); err != nil {
			return err
		}
	}

	return nil
}

func ParseMountOptions(mount *appconfig.Mount, options string) error {
	if options == "" {
		return nil
	}

	pairs := strings.Split(options, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("invalid mount option: %s", pair)
		}
		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "initial_size":
			mount.InitialSize = value
		case "snapshot_retention":
			ret, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid value for snapshot_retention: %s", value)
			}
			mount.SnapshotRetention = &ret
		case "auto_extend_size_threshold":
			threshold, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("invalid value for auto_extend_size_threshold: %s", value)
			}
			mount.AutoExtendSizeThreshold = threshold
		case "auto_extend_size_increment":
			mount.AutoExtendSizeIncrement = value
		case "auto_extend_size_limit":
			mount.AutoExtendSizeLimit = value
		default:
			return fmt.Errorf("unknown mount option: %s", key)
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

	state.appConfig.Compute = state.Plan.Compute

	if state.Plan.HttpServicePort != 0 {
		autostop := fly.MachineAutostopStop
		autostopFlag := flag.GetString(ctx, "auto-stop")

		if autostopFlag == "off" {
			autostop = fly.MachineAutostopOff
		} else if autostopFlag == "suspend" {
			autostop = fly.MachineAutostopSuspend

			// if any compute has a GPU or more than 2GB of memory, set autostop to stop
			for _, compute := range state.appConfig.Compute {
				if compute.MachineGuest != nil && compute.MachineGuest.GPUKind != "" {
					autostop = fly.MachineAutostopStop
					break
				}

				if compute.Memory != "" {
					mb, err := helpers.ParseSize(compute.Memory, units.RAMInBytes, units.MiB)
					if err != nil || mb >= 2048 {
						autostop = fly.MachineAutostopStop
						break
					}
				}
			}
		}

		if state.appConfig.HTTPService == nil {
			state.appConfig.HTTPService = &appconfig.HTTPService{
				ForceHTTPS:         true,
				AutoStartMachines:  fly.Pointer(true),
				AutoStopMachines:   fly.Pointer(autostop),
				MinMachinesRunning: fly.Pointer(0),
				Processes:          []string{"app"},
			}
		}
		state.appConfig.HTTPService.InternalPort = state.Plan.HttpServicePort
	} else {
		state.appConfig.HTTPService = nil
	}
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
