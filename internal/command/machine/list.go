package machine

import (
	"context"
	"fmt"

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
		appName  = appconfig.NameFromContext(ctx)
		io       = iostreams.FromContext(ctx)
		silence  = flag.GetBool(ctx, "quiet")
		cfg      = config.FromContext(ctx)
		colorize = io.ColorScheme()
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

	termWidth := io.TerminalWidth()
	const minWidthForTable = 180 // Width needed to show all columns in table format

	if silence {
		for _, machine := range machines {
			rows = append(rows, []string{colorize.Purple(machine.ID)})
		}
		_ = render.Table(io.Out, "", rows)
	} else {
		unreachableMachines := false

		if termWidth < minWidthForTable {
			// Block layout - each machine as a card
			for i, machine := range machines {
				if i > 0 {
					fmt.Fprintln(io.Out, "================================================================================")
					fmt.Fprintln(io.Out)
				}

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
					note = " *"
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

				checksSummary := "-"
				if checksTotal > 0 {
					checksSummary = fmt.Sprintf("%d/%d", checksPassing, checksTotal)
				}

				// Print machine block
				fmt.Fprintf(io.Out, "Machine %s (%s)%s\n",
					colorize.Purple(machine.ID),
					machine.Name,
					note)

				// Line 1: State, Region, Checks
				fmt.Fprintf(io.Out, "  %s %-12s %s %-6s %s %s",
					colorize.Yellow("State:"),
					machine.State,
					colorize.Yellow("Region:"),
					machine.Region,
					colorize.Yellow("Checks:"),
					lo.Ternary(unreachable, "-", checksSummary))
				if role != "" {
					fmt.Fprintf(io.Out, "  %s %s", colorize.Yellow("Role:"), role)
				}
				fmt.Fprintln(io.Out)

				// Line 2: Image, IP, Volume
				imageRef := lo.Ternary(unreachable, "-", machine.ImageRefWithVersion())
				ipAddr := lo.Ternary(unreachable, "-", machine.PrivateIP)
				if volName == "" {
					volName = "-"
				}
				fmt.Fprintf(io.Out, "  %s %-30s %s %-15s %s %s\n",
					colorize.Yellow("Image:"),
					imageRef,
					colorize.Yellow("IP:"),
					ipAddr,
					colorize.Yellow("Volume:"),
					volName)

				// Line 3: Created, Updated, Process, Size
				created := lo.Ternary(unreachable, "-", machine.CreatedAt)
				updated := lo.Ternary(unreachable, "-", machine.UpdatedAt)
				if machineProcessGroup == "" {
					machineProcessGroup = "-"
				}
				if size == "" {
					size = "-"
				}
				fmt.Fprintf(io.Out, "  %s %-12s %s %-12s %s %-8s %s %s\n",
					colorize.Yellow("Created:"),
					created,
					colorize.Yellow("Updated:"),
					updated,
					colorize.Yellow("Process:"),
					machineProcessGroup,
					colorize.Yellow("Size:"),
					size)
			}

			if unreachableMachines {
				fmt.Fprintln(io.Out)
				fmt.Fprintln(io.Out, "* These Machines' hosts could not be reached.")
			}
		} else {
			// Table layout for wide terminals
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

				machineID := colorize.Purple(machine.ID + note)

				rows = append(rows, []string{
					machineID,
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
			_ = render.Table(io.Out, appName, rows, headers...)

			if unreachableMachines {
				fmt.Fprintln(io.Out, "* These Machines' hosts could not be reached.")
			}
		}
	}
	return nil
}
