package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/chroma/quick"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() *cobra.Command {
	const (
		short = "Show current status of a running machine"
		long  = short + "\n"

		usage = "status <id>"
	)

	cmd := command.New(usage, short, long, runMachineStatus,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)

	cmd.Args = cobra.RangeArgs(0, 1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		selectFlag,
		flag.Bool{
			Name:        "display-config",
			Description: "Display the machine config as JSON",
			Shorthand:   "d",
		},
	)

	return cmd
}

func runMachineStatus(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	machineID := flag.FirstArg(ctx)
	haveMachineID := len(flag.Args(ctx)) > 0
	machine, ctx, err := selectOneMachine(ctx, nil, machineID, haveMachineID)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Machine ID: %s\n", machine.ID)
	fmt.Fprintf(io.Out, "Instance ID: %s\n", machine.InstanceID)
	fmt.Fprintf(io.Out, "State: %s\n\n", machine.State)

	obj := [][]string{
		{
			machine.ID,
			machine.InstanceID,
			machine.State,
			machine.ImageRefWithVersion(),
			machine.Name,
			machine.PrivateIP,
			machine.Region,
			machine.ProcessGroup(),
			fmt.Sprint(machine.Config.Guest.CPUKind),
			fmt.Sprint(machine.Config.Guest.CPUs),
			fmt.Sprint(machine.Config.Guest.MemoryMB),
			machine.CreatedAt,
			machine.UpdatedAt,
			strings.Join(machine.Config.Init.Entrypoint, " "),
			strings.Join(machine.Config.Init.Cmd, " "),
		},
	}

	var cols []string = []string{"ID", "Instance ID", "State", "Image", "Name", "Private IP", "Region", "Process Group", "CPU Kind", "vCPUs", "Memory", "Created", "Updated", "Entrypoint", "Command"}

	if len(machine.Config.Mounts) > 0 {
		cols = append(cols, "Volume")
		obj[0] = append(obj[0], machine.Config.Mounts[0].Volume)
	}

	if err = render.VerticalTable(io.Out, "VM", obj, cols...); err != nil {
		return
	}

	eventLogs := [][]string{}

	for _, event := range machine.Events {
		timeInUTC := time.Unix(0, event.Timestamp*int64(time.Millisecond))
		fields := []string{
			event.Status,
			event.Type,
			event.Source,
			timeInUTC.Format(time.RFC3339Nano),
		}

		if event.Request != nil && event.Request.ExitEvent != nil {
			exitEvent := event.Request.ExitEvent
			fields = append(fields, fmt.Sprintf("exit_code=%d,oom_killed=%t,requested_stop=%t",
				exitEvent.ExitCode, exitEvent.OOMKilled, exitEvent.RequestedStop))
		}

		eventLogs = append(eventLogs, fields)
	}
	_ = render.Table(io.Out, "Event Logs", eventLogs, "State", "Event", "Source", "Timestamp", "Info")

	if flag.GetBool(ctx, "display-config") {
		var prettyConfig []byte
		prettyConfig, err = json.MarshalIndent(machine.Config, "", "  ")

		if err != nil {
			return err
		}

		fmt.Fprint(io.Out, "\nConfig:\n")
		err = quick.Highlight(io.Out, string(prettyConfig), "json", "terminal", "monokai")
		fmt.Fprintln(io.Out)
	}

	return
}
