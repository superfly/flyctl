package appconfig

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func FromRemoteApp(ctx context.Context, appName string) (*Config, error) {
	apiClient := client.FromContext(ctx).API()

	appCompact, err := apiClient.GetAppCompact(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("error getting app: %w", err)
	}

	switch appCompact.PlatformVersion {
	case NomadPlatform:
		serverCfg, err := apiClient.GetConfig(ctx, appName)
		if err != nil {
			return nil, err
		}
		cfg, err := FromDefinition(&serverCfg.Definition)
		if err != nil {
			return nil, err
		}
		if err := cfg.SetNomadPlatform(); err != nil {
			return nil, err
		}
		return cfg, nil
	case MachinesPlatform:
		cfg, err := getAppV2ConfigFromReleases(ctx, apiClient, appCompact.Name)
		if cfg == nil {
			cfg, err = getAppV2ConfigFromMachines(ctx, apiClient, appCompact)
		}
		if err != nil {
			return nil, err
		}
		if err := cfg.SetMachinesPlatform(); err != nil {
			return nil, err
		}
		return cfg, nil
	default:
		if !appCompact.Deployed {
			return nil, fmt.Errorf("Undeployed app '%s' has no platform version set", appName)
		}
		return nil, fmt.Errorf("likely a bug, unknown platform version '%s' for app '%s'. ", appCompact.PlatformVersion, appName)
	}
}

func getAppV2ConfigFromMachines(ctx context.Context, apiClient *api.Client, appCompact *api.AppCompact) (*Config, error) {
	var (
		flapsClient = flaps.FromContext(ctx)
		io          = iostreams.FromContext(ctx)
	)
	activeMachines, err := machine.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing active machines for %s app: %w", appCompact.Name, err)
	}
	machineSet := machine.NewMachineSet(flapsClient, io, activeMachines)
	appConfig, warnings, err := FromAppAndMachineSet(ctx, appCompact, machineSet)
	if err != nil {
		return nil, fmt.Errorf("failed to grab app config from existing machines, error: %w", err)
	}
	if warnings != "" {
		fmt.Fprintf(io.ErrOut, "WARNINGS:\n%s", warnings)
	}
	return appConfig, nil
}

func getAppV2ConfigFromReleases(ctx context.Context, apiClient *api.Client, appName string) (*Config, error) {
	_ = `# @genqlient
	query FlyctlConfigCurrentRelease($appName: String!) {
		app(name:$appName) {
			currentReleaseUnprocessed {
				configDefinition
			}
		}
	}
	`
	resp, err := gql.FlyctlConfigCurrentRelease(ctx, apiClient.GenqClient, appName)
	if err != nil {
		return nil, err
	}
	configDefinition := resp.App.CurrentReleaseUnprocessed.ConfigDefinition
	if configDefinition == nil {
		return nil, nil
	}
	configMapDefinition, err := api.InterfaceToMapOfStringInterface(configDefinition)
	if err != nil {
		return nil, fmt.Errorf("likely a bug, could not convert config definition to api definition error: %w", err)
	}
	apiDefinition := api.DefinitionPtr(configMapDefinition)
	appConfig, err := FromDefinition(apiDefinition)
	if err != nil {
		return nil, fmt.Errorf("error creating appv2 Config from api definition: %w", err)
	}
	return appConfig, err
}
