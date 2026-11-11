package sentry_ext

import (
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

func New() (cmd *cobra.Command) {

	const (
		short = "Provision and manage Sentry projects"
		long  = `Provision and manage Sentry error-tracking projects for your Fly apps.
Each app can have one project. Create it with ` + "`fly ext sentry create`" + `,
then open its errors with ` + "`fly ext sentry dashboard`" + `.`
	)

	cmd = command.New("sentry", short, long, nil)
	cmd.AddCommand(create(), Dashboard(), destroy(), list())

	return cmd
}
