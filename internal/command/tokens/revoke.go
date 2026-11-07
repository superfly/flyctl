package tokens

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/iostreams"
)

func newRevoke() *cobra.Command {
	const (
		short = "Revoke tokens"
		long  = "Revoke one or more tokens."
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
	var failures []error

	args := flag.Args(ctx)
	if len(args) == 0 {
		return fmt.Errorf("no token IDs; please provide IDs as positional arguments")
	}

	for _, id := range args {
		err := apiClient.RevokeLimitedAccessToken(ctx, id)
		if err != nil {
			fmt.Fprintf(io.ErrOut, "failed to revoke token %s: %s\n", id, err)
			failures = append(failures, fmt.Errorf("token %s: %w", id, err))

			continue
		}
		fmt.Fprintf(io.Out, "Revoked %s\n", id)
		numRevoked += 1
	}

	fmt.Fprintf(io.Out, "%d tokens revoked\n", numRevoked)
	if len(failures) > 0 {
		return fmt.Errorf("failed to revoke %d of %d tokens: %w", len(failures), len(args), errors.Join(failures...))
	}

	return nil
}
