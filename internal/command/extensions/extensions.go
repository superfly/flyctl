// Package logs implements the logs command chain.
package extensions

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/extensions/arcjet"
	"github.com/superfly/flyctl/internal/command/extensions/kubernetes"
	sentry_ext "github.com/superfly/flyctl/internal/command/extensions/sentry"
	"github.com/superfly/flyctl/internal/command/extensions/tigris"
	"github.com/superfly/flyctl/internal/command/extensions/vector"
	"github.com/superfly/flyctl/internal/command/extensions/wafris"
)

func New() (cmd *cobra.Command) {
	const (
		long = `Extensions are additional functionality that can be added to your Fly apps`
	)

	cmd = command.New("extensions", long, long, nil)
	cmd.Aliases = []string{"extensions", "ext"}

	cmd.Args = cobra.NoArgs

	cmd.AddCommand(
		sentry_ext.New(),
		tigris.New(),
		kubernetes.New(),
		vector.New(),
		arcjet.New(),
		wafris.New(),
	)

	return
}
