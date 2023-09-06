package scale

import (
	"context"

	"github.com/docker/go-units"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newScaleMemory() *cobra.Command {
	const (
		short = "Set VM memory"
		long  = `Set VM memory to a number of megabytes`
	)
	cmd := command.New("memory [memoryMB]", short, long, runScaleMemory,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.ExactArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.String{
			Name:        "process-group",
			Description: "The process group to apply the VM size to",
			Aliases:     []string{"group"},
		},
	)
	return cmd
}

func runScaleMemory(ctx context.Context) error {
	group := flag.GetString(ctx, "process-group")

	memoryMB, err := helpers.ParseSize(flag.FirstArg(ctx), units.RAMInBytes, units.MiB)
	if err != nil {
		return err
	}

	return scaleVertically(ctx, group, "", memoryMB)
}
