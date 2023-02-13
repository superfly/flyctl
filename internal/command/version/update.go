package version

import (
	"context"
	"errors"
	"fmt"

	"github.com/blang/semver"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/iostreams"
)

func newUpdate() *cobra.Command {
	const (
		short = "Checks for available updates and automatically updates"

		long = `Checks for update and if one is available, runs the appropriate
command to update the application.`
	)

	return command.New("update", short, long, runUpdate)
}

func runUpdate(ctx context.Context) error {
	release, err := update.LatestRelease(ctx, cache.FromContext(ctx).Channel())
	switch {
	case err != nil:
		return fmt.Errorf("failed determining latest release: %w", err)
	case release == nil:
		return fmt.Errorf("failed querying latest release information: %w", err)
	}

	latest, err := semver.ParseTolerant(release.Version)
	if err != nil {
		return fmt.Errorf("error parsing latest release version number %q: %w",
			release.Version, err)
	}

	if buildinfo.Version().GTE(latest) {
		return errors.New("no available update")
	}

	io := iostreams.FromContext(ctx)
	return update.UpgradeInPlace(ctx, io, release.Prerelease)
}
