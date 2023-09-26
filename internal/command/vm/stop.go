package vm

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newStop() *cobra.Command {
	const (
		short = "V1 APPS ONLY: Stop a VM"
		long  = "V1 APPS ONLY: Request for a VM to be asynchronously stopped"
		usage = "stop <vm-id>"
	)

	cmd := command.New(usage, short, long, runStop,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	return cmd
}

func runStop(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		appName = appconfig.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	if err := client.StopAllocation(ctx, appName, flag.FirstArg(ctx)); err != nil {
		return fmt.Errorf("failed to stop allocation: %w", err)
	}

	fmt.Fprintf(io.Out, "VM %s is being stopped\n", flag.FirstArg(ctx))

	return
}
