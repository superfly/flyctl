package agent

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	apiClient "github.com/superfly/flyctl/internal/client"
)

func newInstances() (cmd *cobra.Command) {
	const (
		short = "List instances"
		long  = short + "\n"
		usage = "instances <slug> <app>"
	)

	cmd = command.New(usage, short, long, runInstances,
		command.RequireSession,
	)

	cmd.Args = cobra.ExactArgs(2)

	return
}

func runInstances(ctx context.Context) (err error) {
	var client *agent.Client
	if client, err = establish(ctx); err != nil {
		return
	}

	slug := flag.FirstArg(ctx)
	apiClient := apiClient.FromContext(ctx).API()

	var org *api.Organization
	if org, err = apiClient.FindOrganizationBySlug(ctx, slug); err != nil {
		err = fmt.Errorf("failed fetching org: %w", err)

		return
	}

	app := flag.Args(ctx)[1]

	var instances agent.Instances
	if instances, err = client.Instances(ctx, org, app); err != nil {
		panic(fmt.Sprintf("%#T", err))
		return
	}

	out := iostreams.FromContext(ctx).Out
	err = render.JSON(out, instances)

	return
}
