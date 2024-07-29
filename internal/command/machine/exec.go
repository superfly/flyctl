package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newMachineExec() *cobra.Command {
	const (
		short = "Execute a command on a machine"
		long  = short + "\n"
		usage = "exec [machine-id] <command>"
	)

	cmd := command.New(usage, short, long, runMachineExec,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
		selectFlag,
		flag.Int{
			Name:        "timeout",
			Description: "Timeout in seconds",
		},
	)

	cmd.Args = cobra.RangeArgs(1, 2)

	return cmd
}

func runMachineExec(ctx context.Context) (err error) {
	var (
		args   = flag.Args(ctx)
		io     = iostreams.FromContext(ctx)
		config = config.FromContext(ctx)

		machineID     string
		haveMachineID bool
		command       string
	)

	if len(args) == 2 {
		machineID = args[0]
		haveMachineID = true
		command = args[1]
	} else {
		command = args[0]
	}

	current, ctx, err := selectOneMachine(ctx, "", machineID, haveMachineID)
	if err != nil {
		return err
	}
	flapsClient := flapsutil.ClientFromContext(ctx)

	timeout := flag.GetInt(ctx, "timeout")

	in := &fly.MachineExecRequest{
		Cmd:     command,
		Timeout: timeout,
	}

	out, err := flapsClient.Exec(ctx, current.ID, in)
	if err != nil {
		return fmt.Errorf("could not exec command on machine %s: %w", current.ID, err)
	}

	if config.JSONOutput {
		return render.JSON(io.Out, out)
	}

	if out.ExitCode != 0 {
		fmt.Fprintf(io.Out, "Exit code: %d\n", out.ExitCode)
	}

	if out.StdOut != "" {
		fmt.Fprint(io.Out, out.StdOut)
	}
	if out.StdErr != "" {
		fmt.Fprint(io.ErrOut, out.StdErr)
	}

	return
}
