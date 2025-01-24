package machine

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

const maxStdinByteN = 1 << 20

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
		flag.String{
			Name:        "container",
			Description: "Container name",
		},
	)

	cmd.Args = cobra.RangeArgs(1, 2)

	return cmd
}

func runMachineExec(ctx context.Context) (err error) {
	var (
		args   = flag.Args(ctx)
		ios    = iostreams.FromContext(ctx)
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

	container := flag.GetString(ctx, "container")
	timeout := flag.GetInt(ctx, "timeout")

	var stdin string
	if ios.In != nil {
		b, err := io.ReadAll(io.LimitReader(ios.In, maxStdinByteN))
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
		stdin = string(b)
	}

	in := &fly.MachineExecRequest{
		Cmd:       command,
		Container: container,
		Stdin:     stdin,
		Timeout:   timeout,
	}

	out, err := flapsClient.Exec(ctx, current.ID, in)
	if err != nil {
		return fmt.Errorf("could not exec command on machine %s: %w", current.ID, err)
	}

	if config.JSONOutput {
		return render.JSON(ios.Out, out)
	}

	if out.ExitCode != 0 {
		fmt.Fprintf(ios.Out, "Exit code: %d\n", out.ExitCode)
	}

	if out.StdOut != "" {
		fmt.Fprint(ios.Out, out.StdOut)
	}
	if out.StdErr != "" {
		fmt.Fprint(ios.ErrOut, out.StdErr)
	}

	return
}
