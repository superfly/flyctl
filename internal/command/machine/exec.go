package machine

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newMachineExec() *cobra.Command {

	const (
		short = "Execute a command on a machine"
		long  = short + "\n"
		usage = "exec <machine-id> <command>"
	)

	cmd := command.New(usage, short, long, runMachineExec,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Int{
			Name:        "timeout",
			Description: "Timeout in seconds",
		},
	)

	return cmd
}

func runMachineExec(ctx context.Context) (err error) {
	var (
		appName   = app.NameFromContext(ctx)
		machineID = flag.FirstArg(ctx)
		io        = iostreams.FromContext(ctx)
		config    = config.FromContext(ctx)
	)

	app, err := appFromMachineOrName(ctx, machineID, appName)
	if err != nil {
		return err
	}

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	current, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		return fmt.Errorf("could not retrieve machine %s", machineID)
	}

	var timeout = flag.GetInt(ctx, "timeout")

	in := &api.MachineExecRequest{
		Cmd:     flag.Args(ctx)[1],
		Timeout: timeout,
	}

	out, err := flapsClient.Exec(ctx, current.ID, in)
	if err != nil {
		return fmt.Errorf("could not exec command on machine %s: %w", machineID, err)
	}

	if config.JSONOutput {
		return render.JSON(io.Out, out)
	}

	fmt.Fprintf(io.Out, "Exit code: %d\n", out.ExitCode)
	switch {
	case out.StdOut != nil:
		fmt.Fprintf(io.Out, "Stdout: %s\n", *out.StdOut)
	case out.StdErr != nil:
		fmt.Fprintf(io.Out, "Stderr: %s\n", *out.StdErr)
	}

	return
}
