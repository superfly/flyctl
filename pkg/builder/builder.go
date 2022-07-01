package builder

import (
	"context"
	"os"

	"github.com/superfly/flyctl/api"
)

func RemoteBuilderMachine(ctx context.Context, apiClient *api.Client, appName string) (*api.GqlMachine, *api.App, error) {
	if v := os.Getenv("FLY_REMOTE_BUILDER_HOST"); v != "" {
		return nil, nil, nil
	}

	return apiClient.EnsureRemoteBuilder(ctx, "", appName)
}
