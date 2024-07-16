package appconfig

import (
	"context"
	"fmt"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func FromRemoteApp(ctx context.Context, appName string) (*Config, error) {
	apiClient := flyutil.ClientFromContext(ctx)

	cfg, err := getAppV2ConfigFromReleases(ctx, apiClient, appName)
	if cfg == nil {
		cfg, err = getAppV2ConfigFromMachines(ctx, appName)
	}
	if err != nil {
		return nil, err
	}
	if err := cfg.SetMachinesPlatform(); err != nil {
		return nil, err
	}
	cfg.AppName = appName
	return cfg, nil
}

func getAppV2ConfigFromMachines(ctx context.Context, appName string) (*Config, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)

	activeMachines, err := machine.ListActive(ctx)
	if err != nil {
		return nil, fmt.Errorf("error listing active machines for %s app: %w", appName, err)
	}
	machineSet := machine.NewMachineSet(flapsClient, io, activeMachines, true)
	appConfig, warnings, err := FromAppAndMachineSet(ctx, appName, machineSet)
	if err != nil {
		return nil, fmt.Errorf("failed to grab app config from existing machines, error: %w", err)
	}
	if warnings != "" {
		fmt.Fprintf(io.ErrOut, "WARNINGS:\n%s", warnings)
	}
	return appConfig, nil
}

func getAppV2ConfigFromReleases(ctx context.Context, apiClient flyutil.Client, appName string) (*Config, error) {
	_ = `# @genqlient
	query FlyctlConfigCurrentRelease($appName: String!) {
		app(name:$appName) {
			currentReleaseUnprocessed {
				configDefinition
			}
		}
	}
	`
	resp, err := gql.FlyctlConfigCurrentRelease(ctx, apiClient.GenqClient(), appName)
	if err != nil {
		return nil, err
	}

	configDefinition := resp.App.CurrentReleaseUnprocessed.ConfigDefinition
	if configDefinition == nil {
		return nil, nil
	}

	configMapDefinition, ok := configDefinition.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("likely a bug, could not convert config definition of type %T to api map[string]any", configDefinition)
	}

	appConfig, err := FromDefinition(fly.DefinitionPtr(configMapDefinition))
	if err != nil {
		return nil, fmt.Errorf("error creating appv2 Config from api definition: %w", err)
	}
	return appConfig, err
}
