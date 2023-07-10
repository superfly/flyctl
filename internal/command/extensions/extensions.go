// Package logs implements the logs command chain.
package extensions

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/extensions/planetscale"
	sentry_ext "github.com/superfly/flyctl/internal/command/extensions/sentry"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Extensions are additional functionality that can be added to your Fly apps`
	)

	cmd = command.New("extensions", long, long, nil)
	cmd.Aliases = []string{"extensions", "ext"}

	cmd.Args = cobra.NoArgs

	cmd.AddCommand(sentry_ext.New(), planetscale.New())
	return
}
