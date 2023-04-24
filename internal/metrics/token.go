package metrics

import (
	"context"
	"errors"
	"fmt"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/terminal"
)

func queryMetricsToken(ctx context.Context) (string, error) {

	// Manually construct an API client with the user's access token.
	// We use this over the context API client because we're trying to
	// authenticate the human user, not the specific credentials they're using.
	cfg := config.FromContext(ctx)
	apiClient := client.NewClient(cfg.AccessToken)

	personal, _, err := apiClient.GetCurrentOrganizations(ctx)
	if err != nil {
		return "", err
	}
	if personal.ID == "" {
		return "", errors.New("no personal organization found")
	}

	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient,
		"flyctl-metrics",
		personal.ID,
		"identity",
		struct{}{},
		"",
	)
	if err != nil {
		return "", fmt.Errorf("failed creating identity token: %w", err)
	}
	return resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader, nil
}

func getMetricsToken(parentCtx context.Context) (token string, err error) {
	// Prevent metrics panics from bubbling up to the user.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	cfg := config.FromContext(parentCtx)
	if cfg.MetricsToken != "" {
		terminal.Debugf("Config has metrics token\n")
		return cfg.MetricsToken, nil
	}

	if cfg.MetricsToken == "" && cfg.AccessToken != "" {
		terminal.Debugf("Querying metrics token from web\n")
		token, err := queryMetricsToken(parentCtx)
		if err != nil {
			return "", err
		}
		if err = persistMetricsToken(parentCtx, token); err != nil {
			return "", err
		}
		cfg.MetricsToken = token
		return token, nil
	}
	return "", errors.New("no metrics token in config")
}

func persistMetricsToken(ctx context.Context, token string) error {
	path := state.ConfigFile(ctx)

	if err := config.SetMetricsToken(path, token); err != nil {
		return fmt.Errorf("failed persisting %s in %s: %w\n",
			config.MetricsTokenFileKey, path, err)
	}
	return nil
}
