package redis

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
)

func newCreate() (cmd *cobra.Command) {
	const (
		long = `Create a new Redis instance`

		short = long
		usage = "create"
	)

	cmd = command.New(usage, short, long, runCreate, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
	)

	return cmd
}

func runCreate(ctx context.Context) (err error) {
	var (
		io = iostreams.FromContext(ctx)
	)

	org, err := prompt.Org(ctx)

	if err != nil {
		return
	}

	input := api.CreateAppInput{
		OrganizationID: org.ID,
		Machines:       true,
	}

	app, err := client.FromContext(ctx).
		API().
		CreateApp(ctx, input)

	return err
}
