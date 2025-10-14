package mpg

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

func newBackup() *cobra.Command {
	const (
		short = "Backup commands"
		long  = short + "\n"
	)

	cmd := command.New("backup", short, long, nil)
	cmd.Aliases = []string{"backups"}

	cmd.AddCommand(newBackupList())

	return cmd
}

func newBackupList() *cobra.Command {
	const (
		long  = `List backups for a Managed Postgres cluster.`
		short = "List MPG cluster backups."
		usage = "list [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runBackupList,
		command.RequireSession,
		command.RequireUiex,
	)

	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Bool{
			Name:        "all",
			Description: "Show all backups (default: last 24 hours)",
			Default:     false,
		},
	)

	return cmd
}

func runBackupList(ctx context.Context) error {
	// Check token compatibility early
	if err := validateMPGTokenCompatibility(ctx); err != nil {
		return err
	}

	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	uiexClient := uiexutil.ClientFromContext(ctx)

	clusterID := flag.FirstArg(ctx)
	if clusterID == "" {
		// Should not happen due to cobra.ExactArgs(1), but good practice
		return fmt.Errorf("cluster ID argument is required")
	}

	backups, err := uiexClient.ListManagedClusterBackups(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to list backups for cluster %s: %w", clusterID, err)
	}

	if len(backups.Data) == 0 {
		fmt.Fprintf(out, "No backups found for cluster %s\n", clusterID)
		return nil
	}

	// Filter backups by time (default: last 24 hours)
	var filteredBackups []uiex.ManagedClusterBackup
	showAll := flag.GetBool(ctx, "all")

	if showAll {
		filteredBackups = backups.Data
	} else {
		// Filter to last 24 hours
		cutoff := time.Now().Add(-24 * time.Hour)
		for _, backup := range backups.Data {
			startTime, err := time.Parse(time.RFC3339, backup.Start)
			if err != nil {
				// If we can't parse the time, include the backup
				filteredBackups = append(filteredBackups, backup)
				continue
			}
			if startTime.After(cutoff) {
				filteredBackups = append(filteredBackups, backup)
			}
		}
	}

	if len(filteredBackups) == 0 {
		fmt.Fprintf(out, "No backups found for cluster %s in the last 24 hours (use --all to see all backups)\n", clusterID)
		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(out, filteredBackups)
	}

	rows := make([][]string, 0, len(filteredBackups))
	for _, backup := range filteredBackups {
		rows = append(rows, []string{
			backup.Id,
			backup.Start,
			backup.Status,
			backup.Type,
		})
	}

	return render.Table(out, "", rows, "ID", "Start", "Status", "Type")
}
