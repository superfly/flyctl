package orgs

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newRevoke() *cobra.Command {
	const (
		long = `Revokes an invitation to join an organization that has been sent to a 
user by email.
`
		short = "Revoke a pending invitation to an organization"
		usage = "revoke [org] [email]"
	)

	cmd := command.New(usage, short, long, runRevoke,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func runRevoke(ctx context.Context) (err error) {
	panic("not implemented yet")
}
