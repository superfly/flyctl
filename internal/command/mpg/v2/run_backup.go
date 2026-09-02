package cmdv2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
	"github.com/superfly/flyctl/iostreams"
)

func RunBackupList(ctx context.Context, clusterID string) error {
	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	flapsClient := flapsutil.ClientFromContext(ctx)

	var backups []mpgv2.ClusterBackup
	publicBackups, err := flapsClient.ListManagedPostgresBackups(ctx, clusterID)
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		response, legacyErr := mpgv2.ClientFromContext(ctx).ListClusterBackups(ctx, clusterID)
		if legacyErr != nil {
			return fmt.Errorf("failed to list backups for cluster %s: %w", clusterID, legacyErr)
		}
		backups = response.Data
	} else if err != nil {
		return fmt.Errorf("failed to list backups for cluster %s: %w", clusterID, err)
	} else {
		backups = make([]mpgv2.ClusterBackup, 0, len(publicBackups))
		for _, backup := range publicBackups {
			backups = append(backups, mpgv2.ClusterBackup{
				Id:     backup.ID,
				Status: backup.Status,
				Type:   backup.Type,
				Start:  backup.StartedAt,
				Stop:   backup.FinishedAt,
			})
		}
	}

	if len(backups) == 0 {
		fmt.Fprintf(out, "No backups found for cluster %s\n", clusterID)

		return nil
	}

	// Filter backups by time (default: last 24 hours)
	var filteredBackups []mpgv2.ClusterBackup
	showAll := flag.GetBool(ctx, "all")

	if showAll {
		filteredBackups = backups
	} else {
		// Filter to last 24 hours
		cutoff := time.Now().Add(-24 * time.Hour)
		for _, backup := range backups {
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

func RunBackupCreate(ctx context.Context, clusterID string) error {
	out := iostreams.FromContext(ctx).Out
	flapsClient := flapsutil.ClientFromContext(ctx)

	backupType := flag.GetString(ctx, "type")
	if backupType != "full" && backupType != "incr" {
		return fmt.Errorf("--type must be either 'full' or 'incr'")
	}

	fmt.Fprintf(out, "Creating %s backup for cluster %s...\n", backupType, clusterID)

	err := flapsClient.CreateManagedPostgresBackup(ctx, clusterID, flaps.CreateManagedPostgresBackupRequest{Type: backupType})
	if errors.Is(err, flaps.ErrFlapsNotFound) {
		err = mpgv2.ClientFromContext(ctx).CreateClusterBackup(ctx, clusterID, mpgv2.CreateClusterBackupInput{Type: backupType})
	}
	if err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}

	fmt.Fprintf(out, "Backup queued successfully!\n")

	return nil
}
