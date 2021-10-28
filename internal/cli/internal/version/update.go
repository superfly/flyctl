package version

import (
	"context"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/state"
	"github.com/superfly/flyctl/internal/update"
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
	path := filepath.Join(state.ConfigDirectory(ctx), "state.yml")

	return update.PerformInPlaceUpgrade(ctx, path, buildinfo.Version())
}
