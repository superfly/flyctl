package machine

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/olekukonko/tablewriter/pkg/twwidth"
	"github.com/samber/lo"
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

const defaultMachineListPager = "less -RSX -+F"

func newList() *cobra.Command {
	const (
		short = "List Fly machines"
		long  = short + "\n"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runMachineList,
		command.RequireSession,
		command.RequireAppName,
	)

	cmd.Aliases = []string{"ls"}
	cmd.Args = cobra.NoArgs

	flag.Add(
		cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
		flag.Bool{
			Name:        "quiet",
			Shorthand:   "q",
			Description: "Only list machine ids",
		},
	)

	return cmd
}

func runMachineList(ctx context.Context) (err error) {
	var (
		appName = appconfig.NameFromContext(ctx)
		io      = iostreams.FromContext(ctx)
		silence = flag.GetBool(ctx, "quiet")
		cfg     = config.FromContext(ctx)
	)

	flapsClient := flapsutil.ClientFromContext(ctx)

	machines, err := flapsClient.List(ctx, appName, "")
	if err != nil {
		return err
	}

	if cfg.JSONOutput {
		return render.JSON(io.Out, machines)
	}

	if len(machines) == 0 {
		if !silence {
			fmt.Fprintf(io.Out, "No machines are available on this app %s\n", appName)
		}

		return nil
	}

	rows := [][]string{}

	listOfMachinesLink := io.CreateLink("View them in the UI here", fmt.Sprintf("https://fly.io/apps/%s/machines/", appName))

	if !silence {
		fmt.Fprintf(io.Out, "%d machines have been retrieved from app %s.\n%s\n\n", len(machines), appName, listOfMachinesLink)
	}
	if silence {
		for _, machine := range machines {
			rows = append(rows, []string{machine.ID})
		}
		_ = render.Table(io.Out, "", rows)
	} else {
		unreachableMachines := false

		for _, machine := range machines {
			var volName string
			if machine.Config != nil && len(machine.Config.Mounts) > 0 {
				volName = machine.Config.Mounts[0].Volume
			}

			machineProcessGroup := ""
			size := ""

			if machine.Config != nil {

				if processGroup := machine.ProcessGroup(); processGroup != "" {
					machineProcessGroup = processGroup
				}

				if machine.Config.Guest != nil {
					size = fmt.Sprintf("%s:%dMB", machine.Config.Guest.ToSize(), machine.Config.Guest.MemoryMB)
				}
			}

			note := ""
			unreachable := machine.HostStatus != fly.HostStatusOk
			if unreachable {
				unreachableMachines = true
				note = "*"
			}

			checksTotal := 0
			checksPassing := 0
			role := ""
			for _, c := range machine.Checks {
				checksTotal += 1

				if c.Status == "passing" {
					checksPassing += 1
				}

				if c.Name == "role" {
					role = c.Output
				}
			}

			checksSummary := ""
			if checksTotal > 0 {
				checksSummary = fmt.Sprintf("%d/%d", checksPassing, checksTotal)
			}

			rows = append(rows, []string{
				machine.ID + note,
				machine.Name,
				machine.State,
				lo.Ternary(unreachable, "", checksSummary),
				machine.Region,
				role,
				lo.Ternary(unreachable, "", machine.ImageRefWithVersion()),
				lo.Ternary(unreachable, "", machine.PrivateIP),
				volName,
				lo.Ternary(unreachable, "", machine.CreatedAt),
				lo.Ternary(unreachable, "", machine.UpdatedAt),
				machineProcessGroup,
				size,
			})
		}

		headers := []string{
			"ID",
			"Name",
			"State",
			"Checks",
			"Region",
			"Role",
			"Image",
			"IP Address",
			"Volume",
			"Created",
			"Last Updated",
			"Process Group",
			"Size",
		}

		writeMachineListTable(io, appName, rows, headers)
		if unreachableMachines {
			fmt.Fprintln(io.Out, "* These Machines' hosts could not be reached.")
		}
	}

	return nil
}

func writeMachineListTable(io *iostreams.IOStreams, appName string, rows [][]string, headers []string) {
	if !io.IsInteractive() {
		_ = render.Table(io.Out, appName, rows, headers...)

		return
	}

	var output bytes.Buffer
	_ = render.Table(&output, appName, rows, headers...)

	if shouldPageMachineListTable(output.String(), io.TerminalWidth()) {
		if _, pagerSet := os.LookupEnv("PAGER"); !pagerSet {
			io.SetPager(defaultMachineListPager)
		}
		if err := io.StartPager(); err == nil {
			defer io.StopPager()
		}
	}

	_, _ = io.Out.Write(output.Bytes())
}

func shouldPageMachineListTable(output string, terminalWidth int) bool {
	for line := range strings.SplitSeq(output, "\n") {
		if twwidth.Width(line) > terminalWidth {
			return true
		}
	}

	return false
}
