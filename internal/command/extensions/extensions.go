// Package logs implements the logs command chain.
package extensions

import (
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/extensions/arcjet"
	"github.com/superfly/flyctl/internal/command/extensions/enveloop"
	"github.com/superfly/flyctl/internal/command/extensions/fly_mysql"
	"github.com/superfly/flyctl/internal/command/extensions/kafka"
	"github.com/superfly/flyctl/internal/command/extensions/kubernetes"
	sentry_ext "github.com/superfly/flyctl/internal/command/extensions/sentry"
	"github.com/superfly/flyctl/internal/command/extensions/supabase"
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
		supabase.New(),
		tigris.New(),
		kubernetes.New(),
		kafka.New(),
		vector.New(),
		enveloop.New(),
		arcjet.New(),
		fly_mysql.New(),
		wafris.New(),
	)
	return
}
