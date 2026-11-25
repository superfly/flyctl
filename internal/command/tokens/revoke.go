package tokens

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func newRevoke() *cobra.Command {
	const (
		short = "Revoke API tokens by ID"
		long  = `Revoke one or more API tokens, passed as token IDs. Get IDs from
` + "`fly tokens list`" + `.

If a token can't be revoked, the error is printed and the remaining IDs are
still processed.

To revoke a token you supply with ` + "`--access-token`" + `, ` + "`FLY_ACCESS_TOKEN`" + ` or
` + "`FLY_API_TOKEN`" + `, use ` + "`fly tokens revoke supplied`" + `. To revoke the login saved
by ` + "`fly auth login`" + `, use ` + "`fly auth logout`" + `.`
		usage = "revoke [flags] ID ID ..."
	)

	cmd := command.New(usage, short, long, runRevoke,
		command.RequireSession,
	)
	cmd.AddCommand(newRevokeSupplied())

	return cmd
}

func runRevoke(ctx context.Context) (err error) {
	apiClient := flyutil.ClientFromContext(ctx)
	io := iostreams.FromContext(ctx)

	numRevoked := 0
	failed := 0

	args := flag.Args(ctx)
	if len(args) == 0 {
		return fmt.Errorf("no token IDs; please provide IDs as positional arguments")
	}

	for _, id := range args {
		err := apiClient.RevokeLimitedAccessToken(ctx, id)
		if err != nil {
			fmt.Fprintf(io.ErrOut, "failed to revoke token %s: %s\n", id, err)
			failed++

			continue
		}
		fmt.Fprintf(io.Out, "Revoked %s\n", id)
		numRevoked += 1
	}

	fmt.Fprintf(io.Out, "%d tokens revoked\n", numRevoked)
	if failed > 0 {
		// Each failure is already on stderr with its ID. Repeating them here
		// would print every one twice, and wrapping them would let a single
		// cancelled request choose the exit code.
		return fmt.Errorf("failed to revoke %d of %d tokens", failed, len(args))
	}

	return nil
}
