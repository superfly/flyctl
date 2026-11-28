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
		long  = `Set the memory of every active Machine in a process group. Give the size
in megabytes (such as 2048) or with units (such as 2gb). Each Machine is
updated in turn without a confirmation prompt; fly waits for each one to
settle and, if it's running, to pass its health checks.

Apps with more than one process group need ` + "`--process-group`" + `. If
` + "`fly.toml`" + ` has a ` + "`[[vm]]`" + ` section with a different memory size, fly warns you,
because the next ` + "`fly deploy`" + ` applies the ` + "`fly.toml`" + ` values; update
` + "`fly.toml`" + ` to keep the change.`
	)
	cmd := command.New("memory [memoryMB]", short, long, runScaleMemory,
		command.RequireSession,
		command.RequireAppName,
	)
	cmd.Args = cobra.ExactArgs(1)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.ProcessGroup("The process group to apply the VM size to"),
	)

	return cmd
}

func runScaleMemory(ctx context.Context) error {
	group := flag.GetProcessGroup(ctx)

	memoryMB, err := helpers.ParseSize(flag.FirstArg(ctx), units.RAMInBytes, units.MiB)
	if err != nil {
		return err
	}

	return scaleVertically(ctx, group, "", memoryMB)
}
