package machine

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
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
		long  = `Execute a command on a machine. Pass the remote command as one quoted
argument, for example: fly machine exec <machine-id> 'ls -l /app'.

For non-interactive use, specify the machine ID. Use --timeout to set the
remote command timeout in seconds. With --json, the response contains the
remote exit code and captured stdout and stderr. Otherwise, remote stdout
and stderr are written to the corresponding local streams.

If the remote command exits with a non-zero status, flyctl exits with an error
after printing its output, including when --json is used.`
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
			Description: "Container to run the command in",
		},
		flag.Bool{
			Name:        "no-container",
			Description: "Run the command on the machine itself rather than in one of its containers",
		},
	)

	cmd.Args = cobra.RangeArgs(1, 2)
	cmd.Example = `  fly machine exec <machine-id> 'ls -l /app'
  fly machine exec <machine-id> 'uname -a' --json --timeout 30`

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

	container := flag.GetString(ctx, "container")
	noContainer := flag.GetBool(ctx, "no-container")

	if container != "" && noContainer {
		return errors.New("--container and --no-container are mutually exclusive")
	}

	current, ctx, err := selectOneMachine(ctx, "", machineID, haveMachineID)
	if err != nil {
		return err
	}
	flapsClient := flapsutil.ClientFromContext(ctx)

	// appName is added to context by selectOneMachine
	appName := appconfig.NameFromContext(ctx)

	timeout := flag.GetInt(ctx, "timeout")

	in := &fly.MachineExecRequest{
		Cmd:       command,
		Container: container,
		Machine:   noContainer,
		Timeout:   timeout,
	}

	out, err := flapsClient.Exec(ctx, appName, current.ID, in)
	if err != nil {
		return fmt.Errorf("could not exec command on machine %s: %w", current.ID, err)
	}

	if config.JSONOutput {
		if err := render.JSON(io.Out, out); err != nil {
			return err
		}
	} else {
		fmt.Fprint(io.Out, out.StdOut)
		fmt.Fprint(io.ErrOut, out.StdErr)
	}

	if out.ExitCode != 0 {
		return fmt.Errorf("command on machine %s exited with status %d", current.ID, out.ExitCode)
	}

	return nil
}
