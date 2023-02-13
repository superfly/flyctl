package vm

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newRestart() *cobra.Command {
	const (
		short = "Restart a VM"
		long  = "Request for a VM to be asynchronously restarted."
		usage = "restart <vm-id>"
	)

	cmd := command.New(usage, short, long, runRestart,
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

func runRestart(ctx context.Context) (err error) {
	var (
		io      = iostreams.FromContext(ctx)
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	if err := client.RestartAllocation(ctx, appName, flag.FirstArg(ctx)); err != nil {
		return fmt.Errorf("failed to restart allocation: %w", err)
	}

	fmt.Fprintf(io.Out, "VM %s is being restarted\n", flag.FirstArg(ctx))

	return
}
