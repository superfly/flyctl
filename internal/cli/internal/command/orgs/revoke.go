package orgs

import (
	"context"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/cli/internal/command"
)

func newRevoke() *cobra.Command {
	const (
		long = `Revokes an invitation to join an organization that has been sent to a 
user by email.
`
		short = "Revoke a pending invitation to an organization"
		usage = "revoke [slug] [email]"
	)

	cmd := command.New(usage, short, long, runRevoke,
		command.RequireSession)

	cmd.Args = cobra.MaximumNArgs(2)

	return cmd
}

func runRevoke(ctx context.Context) (err error) {
	// TODO: this has no corresponding endpoint
	return errors.New("not implemented yet")
}
