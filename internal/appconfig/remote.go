package appconfig

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"

	"github.com/Khan/genqlient/graphql"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

func FromRemoteApp(ctx context.Context, appName string) (*Config, error) {
	return FromRemoteAppForGroups(ctx, appName, nil)
}

// FromRemoteAppForGroups verifies groups omitted by Machine reconstruction against
// the release. Callers still validate group names in an authoritative release config.
func FromRemoteAppForGroups(ctx context.Context, appName string, groups []string) (*Config, error) {
	apiClient := flyutil.ClientFromContext(ctx)

	cfg, err := getAppV2ConfigFromReleases(ctx, apiClient, appName)
	if cfg == nil {
		cfg, err = fallbackConfigForGroups(ctx, apiClient, appName, groups, err)
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

func fallbackConfigForGroups(ctx context.Context, client flyutil.Client, appName string, groups []string, releaseErr error) (*Config, error) {
	cfg, err := getAppV2ConfigFromMachines(ctx, appName)
	if err != nil || len(groups) == 0 {
		return cfg, err
	}
	names := cfg.ProcessNames()
	for _, group := range groups {
		if slices.Contains(names, group) {
			continue
		}
		if ctx.Err() == nil && transientReleaseError(releaseErr) {
			// Limit recovery to one read, including transport-level retries.
			cfg, releaseErr = getAppV2ConfigFromReleases(fly.WithoutHTTPRetries(ctx), client, appName)
			if cfg != nil && releaseErr == nil {
				return cfg, nil
			}
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		reason := "the release has no stored config"
		if releaseErr != nil {
			reason = "the release config could not be loaded"
		}
		return nil, fmt.Errorf("could not verify process group %q: %s and the config reconstructed from Machines only lists %v", group, reason, names)
	}
	return cfg, nil
}

func transientReleaseError(err error) bool {
	var httpErr *graphql.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		}
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func getAppV2ConfigFromMachines(ctx context.Context, appName string) (*Config, error) {
	flapsClient := flapsutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)

	activeMachines, err := machine.ListActive(ctx, appName)
	if err != nil {
		return nil, fmt.Errorf("error listing active machines for %s app: %w", appName, err)
	}
	machineSet := machine.NewMachineSet(flapsClient, io, appName, activeMachines, true)
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
