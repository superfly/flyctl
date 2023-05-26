package tokens

import (
	"context"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newRevoke() *cobra.Command {
	const (
		short = "Revoke tokens"
		long  = "used like: 'fly tokens revoke [ids]'"
		usage = "revoke"
	)

	cmd := command.New(usage, short, long, runRevoke,
		command.RequireSession,
	)

	return cmd
}

func runRevoke(ctx context.Context) (err error) {
	apiClient := client.FromContext(ctx).API()

	args := flag.Args(ctx)
	for _, id := range args {
		err := apiClient.RevokeLimitedAccessToken(ctx, id)
		if err != nil {
			fmt.Printf("Failed to revoke token %s: %s\n", id, err)
			continue
		}
		fmt.Printf("Revoked %s\n", id)
	}

	return nil
}
