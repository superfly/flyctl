package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alecthomas/chroma/quick"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/pkg/flaps"
	"github.com/superfly/flyctl/pkg/iostreams"
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

	cmd.Args = cobra.ExactArgs(1)

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.Bool{
			Name:        "display-config",
			Description: "Display the machine config as JSON",
			Shorthand:   "d",
		},
	)

	return cmd
}

func runMachineStatus(ctx context.Context) (err error) {
	var (
		io     = iostreams.FromContext(ctx)
		client = client.FromContext(ctx).API()
	)

	var (
		appName   = app.NameFromContext(ctx)
		machineID = flag.FirstArg(ctx)
	)

	// flaps client
	if appName == "" {
		return fmt.Errorf("app is not found")
	}
	app, err := client.GetAppCompact(ctx, appName)
	if err != nil {
		return err
	}
	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return fmt.Errorf("could not make flaps client: %w", err)
	}

	machine, err := flapsClient.Get(ctx, machineID)
	if err != nil {
		switch {
		case strings.Contains(err.Error(), "status"):
			return fmt.Errorf("retrieve machine failed %s", err)
		default:
			return fmt.Errorf("machine %s could not be retrieved", machineID)
		}
	}

	eventLogs := [][]string{}

	fmt.Fprintf(io.Out, "Success! A machine has been retrieved\n")
	fmt.Fprintf(io.Out, " Machine ID: %s\n", machine.ID)
	fmt.Fprintf(io.Out, " Instance ID: %s\n", machine.InstanceID)
	fmt.Fprintf(io.Out, " State: %s\n\n", machine.State)
	for _, event := range machine.Events {
		timeInUTC := time.Unix(0, event.Timestamp*int64(time.Millisecond))
		eventLogs = append(eventLogs, []string{
			event.Status,
			event.Type,
			event.Source,
			timeInUTC.Format(time.RFC3339Nano),
		})
	}
	_ = render.Table(io.Out, "Event Logs", eventLogs, "Machine Status", "Event Type", "Source", "Timestamp")

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
