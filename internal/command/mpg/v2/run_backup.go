package cmdv2

import (
	"context"
	"fmt"
	"time"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

// Copied from v1 RunBackupList and changed client to mpgv2
func RunBackupList(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv2.ClientFromContext(ctx)

	backups, err := mpgClient.ListManagedClusterBackups(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed to list backups for cluster %s: %w", clusterID, err)
	}

	if len(backups.Data) == 0 {
		fmt.Fprintf(out, "No backups found for cluster %s\n", clusterID)

		return nil
	}

	// Filter backups by time (default: last 24 hours)
	var filteredBackups []mpgv2.ManagedClusterBackup
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

// Copied from v1 RunBackupCreate and changed client to mpgv2
func RunBackupCreate(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out
	mpgClient := mpgv2.ClientFromContext(ctx)

	backupType := flag.GetString(ctx, "type")
	if backupType != "full" && backupType != "incr" {
		return fmt.Errorf("--type must be either 'full' or 'incr'")
	}

	fmt.Fprintf(out, "Creating %s backup for cluster %s...\n", backupType, clusterID)

	input := mpgv2.CreateManagedClusterBackupInput{
		Type: backupType,
	}

	response, err := mpgClient.CreateManagedClusterBackup(ctx, clusterID, input)
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	fmt.Fprintf(out, "Backup queued successfully!\n")
	fmt.Fprintf(out, "  ID: %s\n", response.Data.Id)

	return nil
}
