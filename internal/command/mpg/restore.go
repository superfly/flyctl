package mpg

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newRestore() *cobra.Command {
	const (
		long  = `Restore a Managed Postgres cluster from a backup.`
		short = "Restore MPG cluster from backup."
		usage = "restore [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runRestore,
		command.RequireSession,
		command.RequireUiex,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.String{
			Name:        "backup-id",
			Description: "The backup ID to restore from",
		},
	)

	return cmd
}

func runRestore(ctx context.Context) error {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		// Should not happen due to cobra.ExactArgs(1), but good practice
		return fmt.Errorf("cluster ID argument is required")
	}

	backupID := flag.GetString(ctx, "backup-id")
	if backupID == "" {
		return fmt.Errorf("--backup-id flag is required")
	}

	// TODO: Implement restore functionality
	fmt.Fprintf(out, "Restoring cluster %s from backup %s...\n", clusterID, backupID)
	fmt.Fprintf(out, "(Restore functionality not yet implemented)\n")

	return nil
}
