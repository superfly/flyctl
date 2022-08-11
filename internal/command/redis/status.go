package redis

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() *cobra.Command {
	const (
		short = "Show status of a Redis service"
		long  = short + "\n"

		usage = "status <id>"
	)

	cmd := command.New(usage, short, long, runStatus,
		command.RequireSession,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd)

	return cmd
}

func runStatus(ctx context.Context) (err error) {
	var (
		io     = iostreams.FromContext(ctx)
		id     = flag.FirstArg(ctx)
		client = client.FromContext(ctx).API()
	)

	service, err := client.GetAddOn(ctx, id)
	if err != nil {
		return err
	}

	obj := [][]string{
		{
			service.ID,
			service.Name,
			service.PrimaryRegion,
			service.PublicUrl,
		},
	}

	var cols []string = []string{"ID", "Name", "Primary Region", "Public URL"}

	if err = render.VerticalTable(io.Out, "Redis", obj, cols...); err != nil {
		return
	}

	return
}
