package uiexutil

import (
	"context"

	"github.com/superfly/flyctl/internal/uiex"
)

// LatestRelease returns the app's most recent release regardless of status,
// matching the GraphQL currentReleaseUnprocessed field, or nil when the app
// has no releases yet.
//
// It deliberately uses the release list rather than the releases/current
// endpoint: that endpoint only considers complete releases, and fails for
// apps that have none.
func LatestRelease(ctx context.Context, client Client, appName string) (*uiex.Release, error) {
	releases, err := client.ListReleases(ctx, appName, 1)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, nil
	}

	return &releases[0], nil
}
