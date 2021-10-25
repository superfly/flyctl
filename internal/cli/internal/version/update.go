package version

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newUpdate() *cobra.Command {
	return command.FromDocstrings("version.update", runUpdate)
}

func runUpdate(context.Context) error {
	return nil
}
