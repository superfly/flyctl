package tokens

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
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

	return cmd
}

func runRevoke(ctx context.Context) (err error) {
	apiClient := flyutil.ClientFromContext(ctx)

	numRevoked := 0

	args := flag.Args(ctx)
	if len(args) == 0 {
		return fmt.Errorf("no token IDs; please provide IDs as positional arguments")
	}

	for _, id := range args {
		err := apiClient.RevokeLimitedAccessToken(ctx, id)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to revoke token %s: %s\n", id, err)
			continue
		}
		fmt.Printf("Revoked %s\n", id)
		numRevoked += 1
	}

	fmt.Printf("%d tokens revoked\n", numRevoked)

	return nil
}
