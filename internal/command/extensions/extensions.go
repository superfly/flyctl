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
		short = "Provision and manage extensions for Fly apps"
		long  = `Provision and manage services from Fly.io extension partners, such as
Tigris object storage, Sentry, Upstash Vector, Arcjet and Wafris, as well as
Fly Kubernetes clusters. Many extensions can be attached to an app, which
then gets the extension's credentials as secrets.

Each extension has its own subcommand, such as ` + "`fly ext storage`" + ` or
` + "`fly ext sentry`" + `. Most start with a create command.`
	)

	cmd = command.New("extensions", short, long, nil)
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
