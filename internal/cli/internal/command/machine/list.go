package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/flag"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func newList() *cobra.Command {
	const (
		short = "List machines"
		long  = short + "\n"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runMachineList,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.NoArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "all",
			Description: "Show machines in all states",
		},
		flag.String{
			Name:        "state",
			Default:     "started",
			Description: "List machines in a specific state.",
		},
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Only list machine ids",
		},
	)

	return cmd
}

func runMachineList(ctx context.Context) (err error) {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
		stream  = iostreams.FromContext(ctx)
		cfg     = config.FromContext(ctx)
	)

	state := flag.GetString(ctx, "state")
	if flag.GetBool(ctx, "all") {
		state = ""
	}

	machines, err := client.ListMachines(ctx, appName, state)
	if err != nil {
		return fmt.Errorf("could not get lisyt of machines: %w", err)
	}

	if flag.GetBool(ctx, "quiet") {
		for _, machine := range machines {
			fmt.Fprintf(stream.Out, "%s\n", machine.ID)
		}
		return nil
	}

	if cfg.JSONOutput {
		return render.JSON(stream.Out, machines)
	}

	rows := [][]string{}

	for _, machine := range machines {
		var ipv6 string

		for _, ip := range machine.IPs.Nodes {
			if ip.Family == "v6" && ip.Kind == "privatenet" {
				ipv6 = ip.IP
			}
		}

		rows = append(rows, []string{
			machine.ID,
			machine.Config.Image,
			machine.CreatedAt.String(),
			machine.State,
			machine.Region,
			machine.Name,
			ipv6,
		})

	}
	_ = render.Table(stream.Out, appName, rows, "ID", "Image", "Created", "State", "Region", "Name", "IP Address")

	return
}
