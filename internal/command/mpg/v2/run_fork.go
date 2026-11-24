package cmdv2

import (
	"context"
	"fmt"

	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mpgutil"
	"github.com/superfly/flyctl/iostreams"
)

// RunFork forks clusterID into a new cluster. An empty name or plan is left
// out of the request so the API applies its defaults: the source's name with
// a -fork suffix, and the source cluster's plan.
func RunFork(ctx context.Context, clusterID, name, plan string) error {
	io := iostreams.FromContext(ctx)
	jsonOutput := config.FromContext(ctx).JSONOutput
	out := io.Out
	if jsonOutput {
		out = io.ErrOut
	}

	fmt.Fprintf(out, "Forking cluster %s...\n", clusterID)

	request := flaps.ForkManagedPostgresClusterRequest{
		Name: name,
		Plan: plan,
	}

	response, err := flapsutil.ClientFromContext(ctx).ForkManagedPostgresCluster(ctx, clusterID, request)
	if err != nil {
		return fmt.Errorf("failed to fork cluster: %w", err)
	}

	if jsonOutput {
		return mpgutil.RenderCreatedJSON(io.Out, fmt.Sprintf("cluster %s", response.ID), mpgutil.ClusterResult{
			ID: response.ID, Name: response.Name,
			Organization: mpgutil.ClusterOrganization{Name: response.Organization.Name, Slug: response.Organization.Slug},
			Region:       response.Region, Plan: response.Plan, Status: response.Status,
		})
	}

	fmt.Fprintf(out, "Fork initiated successfully!\n")
	fmt.Fprintf(out, "  Cluster ID: %s\n", response.ID)
	fmt.Fprintf(out, "  Cluster Name: %s\n", response.Name)
	if response.Plan != "" {
		fmt.Fprintf(out, "  Plan: %s\n", response.Plan)
	}

	return nil
}
