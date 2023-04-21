package extensions

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func newSentry() (cmd *cobra.Command) {

	const (
		short = "Setup a Sentry project for this app"
		long  = short + "\n"
	)

	cmd = command.New("sentry", short, long, nil)
	cmd.AddCommand(newSentryCreate())

	return cmd
}
