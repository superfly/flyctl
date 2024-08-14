package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/alecthomas/chroma/quick"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() *cobra.Command {
	const (
		short = "Show current status of a running machine"
		long  = short + "\n"

		usage = "status [id]"
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

func optJsonStrings(v []string) string {
	if len(v) > 0 {
		bytes, _ := json.Marshal(v)
		return string(bytes)
	} else {
		return ""
	}
}

func runMachineStatus(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)

	machineID := flag.FirstArg(ctx)
	haveMachineID := len(flag.Args(ctx)) > 0
	machine, ctx, err := selectOneMachine(ctx, "", machineID, haveMachineID)
	if err != nil {
		return err
	}

	checksRows := [][]string{}
	checksTotal := 0
	checksPassing := 0
	roleOutput := ""
	for _, c := range machine.Checks {
		checksTotal += 1

		if c.Status == "passing" {
			checksPassing += 1
		}

		if c.Name == "role" && c.Status == "passing" {
			roleOutput = c.Output
		}

		fields := []string{
			c.Name,
			string(c.Status),
			format.RelativeTime(*c.UpdatedAt),
			c.Output,
		}
		checksRows = append(checksRows, fields)
	}

	checksSummary := ""
	if checksTotal > 0 {
		checksSummary = fmt.Sprintf("%d/%d", checksPassing, checksTotal)
	}

	mConfig := machine.GetConfig()

	fmt.Fprintf(io.Out, "Machine ID: %s\n", machine.ID)
	fmt.Fprintf(io.Out, "Instance ID: %s\n", machine.InstanceID)
	fmt.Fprintf(io.Out, "State: %s\n", machine.State)
	fmt.Fprintf(io.Out, "HostStatus: %s\n", machine.HostStatus)
	fmt.Fprintf(io.Out, "\n")

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
			fmt.Sprint(mConfig.Guest.CPUKind),
			fmt.Sprint(mConfig.Guest.CPUs),
			fmt.Sprint(mConfig.Guest.MemoryMB),
			machine.CreatedAt,
			machine.UpdatedAt,
			optJsonStrings(mConfig.Init.Entrypoint),
			optJsonStrings(mConfig.Init.Cmd),
		},
	}

	var cols []string = []string{"ID", "Instance ID", "State", "Image", "Name", "Private IP", "Region", "Process Group", "CPU Kind", "vCPUs", "Memory", "Created", "Updated", "Entrypoint", "Command"}

	if len(mConfig.Mounts) > 0 {
		cols = append(cols, "Volume")
		obj[0] = append(obj[0], mConfig.Mounts[0].Volume)
	}

	if err = render.VerticalTable(io.Out, "VM", obj, cols...); err != nil {
		return
	}

	if mConfig.Metadata["fly-managed-postgres"] == "true" {
		obj := [][]string{
			{
				roleOutput,
			},
		}
		_ = render.VerticalTable(io.Out, "PG", obj, "Role")
	}

	checksTableTitle := fmt.Sprintf("Checks [%s]", checksSummary)
	if len(checksRows) > 0 {
		_ = render.Table(io.Out, checksTableTitle, checksRows, "Name", "Status", "Last Updated", "Output")
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

		// This is terrible but will inform the users good enough while I build something
		// elegant like the ExitEvent above
		if event.Type == "launch" && event.Status == "created" && event.Source == "flyd" {
			fields = append(fields, "migrated=true")
		}

		eventLogs = append(eventLogs, fields)
	}
	_ = render.Table(io.Out, "Event Logs", eventLogs, "State", "Event", "Source", "Timestamp", "Info")

	if flag.GetBool(ctx, "display-config") {
		var prettyConfig []byte
		prettyConfig, err = json.MarshalIndent(mConfig, "", "  ")

		if err != nil {
			return err
		}

		fmt.Fprint(io.Out, "\nConfig:\n")
		err = quick.Highlight(io.Out, string(prettyConfig), "json", "terminal", "monokai")
		fmt.Fprintln(io.Out)
	}

	return
}
