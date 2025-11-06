package mpg

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/uiexutil"
)

func newStatus() *cobra.Command {
	const (
		long  = `Show status and details of a specific Managed Postgres cluster using its ID.`
		short = "Show MPG cluster status."
		usage = "status [CLUSTER_ID]"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession,
		command.RequireUiex,
	)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runStatus(ctx context.Context) error {
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

	// Fetch detailed cluster information by ID
	clusterDetails, err := uiexClient.GetManagedClusterById(ctx, clusterID)
	if err != nil {
		return fmt.Errorf("failed retrieving details for cluster %s: %w", clusterID, err)
	}

	if cfg.JSONOutput {
		return render.JSON(out, clusterDetails)
	}

	rows := [][]string{{
		clusterDetails.Data.Id,
		clusterDetails.Data.Name,
		clusterDetails.Data.Organization.Slug,
		clusterDetails.Data.Region,
		clusterDetails.Data.Status,
		strconv.Itoa(clusterDetails.Data.Disk),
		strconv.Itoa(clusterDetails.Data.Replicas),
		clusterDetails.Data.IpAssignments.Direct,
	}}

	cols := []string{
		"ID",
		"Name",
		"Organization",
		"Region",
		"Status",
		"Allocated Disk (GB)",
		"Replicas",
		"Direct IP",
	}

	return render.VerticalTable(out, "Cluster Status", rows, cols...)
}
