package machine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/AlecAivazis/survey/v2/terminal"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

const (
	defaultMachineListPageSize = 500

	machineListQuit     machineListAction = 'q'
	machineListNextPage machineListAction = 'n'
	machineListPrevPage machineListAction = 'p'
)

type machineListAction rune

// machineListCachedPage caches pages of machines when paginating through a
// large result set.
type machineListCachedPage struct {
	machines   []*fly.Machine
	nextCursor string
}

// machineListNavigation captures where we are in the paginated machine list.
type machineListNavigation struct {
	hasNext bool
	hasPrev bool
	page    int
}

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
		flag.Int{
			Name:        "limit",
			Description: "Number of machines to return per page; 0 returns all machines",
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
		limit   = flag.GetInt(ctx, "limit")
	)
	if limit < 0 {
		return fmt.Errorf("--limit must be 0 or greater")
	}

	flapsClient := flapsutil.ClientFromContext(ctx)
	pageLister, ok := flapsClient.(machinePageLister)
	if !ok {
		return fmt.Errorf("Machines API client does not support paginated machine lists")
	}

	seenMachines := make(map[string]struct{})
	seenCursors := make(map[string]struct{})
	machines, nextCursor, err := loadMachineDisplayPage(ctx, pageLister, appName, limit, "", seenMachines, seenCursors)
	if err != nil {
		return err
	}

	interactivePagination := limit > 0 && io.IsInteractive() && !silence && !cfg.JSONOutput && nextCursor != ""
	if !interactivePagination {
		_, err := renderMachineListPage(io, appName, machines, silence, cfg.JSONOutput, nil)
		return err
	}

	pages := []machineListCachedPage{
		{machines: machines, nextCursor: nextCursor},
	}
	pageIndex := 0
	for {
		page := pages[pageIndex]
		action, err := renderMachineListPage(io, appName, page.machines, silence, cfg.JSONOutput, &machineListNavigation{
			hasNext: page.nextCursor != "",
			hasPrev: pageIndex > 0,
			page:    pageIndex + 1,
		})
		if err != nil {
			return err
		}

		switch action {
		case machineListQuit:
			return nil
		case machineListPrevPage:
			if pageIndex == 0 {
				continue
			}
			pageIndex--
		case machineListNextPage:
			if pageIndex+1 < len(pages) {
				pageIndex++
			} else if page.nextCursor != "" {
				machines, nextCursor, err := loadMachineDisplayPage(ctx, pageLister, appName, limit, page.nextCursor, seenMachines, seenCursors)
				if err != nil {
					return err
				}
				if len(machines) == 0 {
					pages[pageIndex].nextCursor = ""
					continue
				}
				pages = append(pages, machineListCachedPage{machines: machines, nextCursor: nextCursor})
				pageIndex++
			} else {
				continue
			}
		default:
			return nil
		}

		clearMachineListPage(io.Out)
	}
}

func renderMachineListPage(io *iostreams.IOStreams, appName string, machines []*fly.Machine, silence, jsonOutput bool, navigation *machineListNavigation) (machineListAction, error) {
	if jsonOutput {
		return machineListQuit, render.JSON(io.Out, machines)
	}

	if len(machines) == 0 {
		if !silence {
			fmt.Fprintf(io.Out, "No machines are available on this app %s\n", appName)
		}

		return machineListQuit, nil
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

		footer := ""
		if unreachableMachines {
			footer = "* These Machines' hosts could not be reached."
		}
		return writeMachineListTable(io, appName, rows, headers, footer, navigation)
	}

	return machineListQuit, nil
}

type machinePageLister interface {
	ListMachines(context.Context, string, *flaps.ListMachinesOpts) (*flaps.ListMachinesResponse, error)
}

func loadMachineDisplayPage(ctx context.Context, client machinePageLister, appName string, limit int, cursor string, seenMachines, seenCursors map[string]struct{}) ([]*fly.Machine, string, error) {
	initialCapacity := defaultMachineListPageSize
	if limit > 0 {
		initialCapacity = min(limit, defaultMachineListPageSize)
	}
	machines := make([]*fly.Machine, 0, initialCapacity)

	for {
		if cursor != "" {
			if _, ok := seenCursors[cursor]; ok {
				return nil, "", fmt.Errorf("Machines API returned a repeated pagination cursor")
			}
			seenCursors[cursor] = struct{}{}
		}
		pageSize := defaultMachineListPageSize
		if limit > 0 {
			pageSize = min(pageSize, limit-len(machines))
		}
		resp, err := client.ListMachines(ctx, appName, &flaps.ListMachinesOpts{
			Limit:  pageSize,
			Cursor: cursor,
		})
		if err != nil {
			return nil, "", err
		}

		for _, machine := range resp.Machines {
			if _, ok := seenMachines[machine.ID]; ok {
				continue
			}
			seenMachines[machine.ID] = struct{}{}
			machines = append(machines, machine)
			// We have all the machines we need to display the next machine
			// page: return the batch.
			if limit > 0 && len(machines) == limit {
				return machines, resp.NextCursor, nil
			}
		}
		// This is the last batch of machines.
		if resp.NextCursor == "" {
			return machines, "", nil
		}
		cursor = resp.NextCursor
	}
}

func writeMachineListTable(io *iostreams.IOStreams, appName string, rows [][]string, headers []string, footer string, navigation *machineListNavigation) (machineListAction, error) {
	if navigation == nil || !io.IsInteractive() {
		_ = render.Table(io.Out, appName, rows, headers...)
		if footer != "" {
			fmt.Fprintln(io.Out, footer)
		}

		return machineListQuit, nil
	}

	var output bytes.Buffer
	_ = render.Table(&output, appName, rows, headers...)
	if footer != "" {
		fmt.Fprintln(&output, footer)
	}

	io.SetPager(machineListNavigationPager(*navigation))
	if err := io.StartPager(); err != nil {
		_, _ = io.Out.Write(output.Bytes())
		return readMachineListNavigation(io, *navigation)
	}
	if _, err := io.Out.Write(output.Bytes()); err != nil {
		io.StopPager()
		return machineListQuit, err
	}

	// Use the pager's exit code to figure out the next action.
	exitCode := io.StopPagerWithExitCode()
	if exitCode == 1 {
		_, _ = io.Out.Write(output.Bytes())
		return readMachineListNavigation(io, *navigation)
	}

	return machineListActionFromExitCode(exitCode), nil
}

func machineListNavigationPager(navigation machineListNavigation) string {
	return fmt.Sprintf("less --lesskey-content=%s -RSX -+F -P%s",
		strconv.Quote(machineListNavigationKeys(navigation)),
		strconv.Quote(machineListNavigationPrompt(navigation, true)),
	)
}

func machineListNavigationKeys(navigation machineListNavigation) string {
	// Create custom keybindings for the pager, so we can figure out what key
	// the user pressed.
	bindings := []string{"#command", "q quit q"}
	if navigation.hasNext {
		bindings = append(bindings, "n quit n")
	}
	if navigation.hasPrev {
		bindings = append(bindings, "p quit p")
	}

	return strings.Join(bindings, ";")
}

func readMachineListNavigation(io *iostreams.IOStreams, navigation machineListNavigation) (machineListAction, error) {
	in, inOK := io.In.(terminal.FileReader)
	out, outOK := io.Out.(terminal.FileWriter)
	if !inOK || !outOK {
		return machineListQuit, nil
	}

	reader := terminal.NewRuneReader(terminal.Stdio{In: in, Out: out, Err: io.ErrOut})
	if err := reader.SetTermMode(); err != nil {
		return machineListQuit, nil
	}
	defer reader.RestoreTermMode() //nolint:errcheck

	menu := machineListNavigationPrompt(navigation, false)
	fmt.Fprint(io.Out, menu)
	defer fmt.Fprintln(io.Out)
	for {
		key, _, err := reader.ReadRune()
		if err != nil {
			return machineListQuit, err
		}
		switch machineListAction(key) {
		case machineListNextPage:
			if navigation.hasNext {
				return machineListNextPage, nil
			}
		case machineListPrevPage:
			if navigation.hasPrev {
				return machineListPrevPage, nil
			}
		case machineListQuit, machineListAction(terminal.KeyInterrupt), machineListAction(terminal.KeyEscape):
			return machineListQuit, nil
		}
	}
}

func machineListNavigationPrompt(navigation machineListNavigation, canScroll bool) string {
	actions := []string{fmt.Sprintf("page %d", navigation.page)}
	if navigation.hasNext {
		actions = append(actions, "[n] next")
	}
	if navigation.hasPrev {
		actions = append(actions, "[p] previous")
	}
	actions = append(actions, "[q] quit")
	if canScroll {
		actions = append(actions, "arrows scroll")
	}

	return strings.Join(actions, ", ")
}

func machineListActionFromExitCode(exitCode int) machineListAction {
	switch machineListAction(exitCode) {
	case machineListNextPage:
		return machineListNextPage
	case machineListPrevPage:
		return machineListPrevPage
	default:
		return machineListQuit
	}
}

func clearMachineListPage(out io.Writer) {
	// Clear the terminal screen and move cursor to the top-left corner.
	_, _ = io.WriteString(out, "\x1b[2J\x1b[H")
}
