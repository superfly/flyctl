package scale

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newScaleShow() *cobra.Command {
	const (
		short = ""
		long  = ""
	)
	cmd := command.New("show", short, long, runScaleShow,
		command.RequireSession,
		command.RequireAppName,
		failOnMachinesApp,
	)
	cmd.Args = cobra.NoArgs
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
	)
	cmd.AddCommand()
	return cmd
}

func runScaleShow(ctx context.Context) error {
	apiClient := client.FromContext(ctx).API()
	appName := appconfig.NameFromContext(ctx)

	size, tgCounts, processGroups, err := apiClient.AppVMResources(ctx, appName)
	if err != nil {
		return err
	}

	countMsg := countMessage(tgCounts)
	maxPerRegionMsg := maxPerRegionMessage(processGroups)
	printVMResources(ctx, size, countMsg, maxPerRegionMsg, processGroups)
	return nil
}

func countMessage(counts []api.TaskGroupCount) string {
	msg := ""

	if len(counts) == 1 {
		for _, tg := range counts {
			if tg.Name == "app" {
				msg = fmt.Sprint(tg.Count)
			}
		}
	}

	if msg == "" {
		for _, tg := range counts {
			msg += fmt.Sprintf("%s=%d ", tg.Name, tg.Count)
		}
	}

	return msg
}

func maxPerRegionMessage(groups []api.ProcessGroup) string {
	msg := ""

	if len(groups) == 1 {
		for _, pg := range groups {
			if pg.Name == "app" {
				if pg.MaxPerRegion == 0 {
					msg = "Not set"
				} else {
					msg = fmt.Sprint(pg.MaxPerRegion)
				}
			}
		}
	}

	if msg == "" {
		for _, pg := range groups {
			msg += fmt.Sprintf("%s=%d ", pg.Name, pg.MaxPerRegion)
		}
	}

	return msg
}

func printVMResources(ctx context.Context, vmSize api.VMSize, count string, maxPerRegion string, processGroups []api.ProcessGroup) {
	io := iostreams.FromContext(ctx)
	appName := appconfig.NameFromContext(ctx)

	if flag.GetBool(ctx, "json") {
		out := struct {
			api.VMSize
			Count        string
			MaxPerRegion string
		}{
			VMSize:       vmSize,
			Count:        count,
			MaxPerRegion: maxPerRegion,
		}

		prettyJSON, _ := json.MarshalIndent(out, "", "    ")
		fmt.Fprintln(io.Out, string(prettyJSON))
		return
	}

	fmt.Printf("VM Resources for %s\n", appName)

	if len(processGroups) <= 1 {
		fmt.Fprintf(io.Out, "%15s: %s\n", "VM Size", vmSize.Name)
		fmt.Fprintf(io.Out, "%15s: %s\n", "VM Memory", formatMemory(vmSize))
	}

	fmt.Fprintf(io.Out, "%15s: %s\n", "Count", count)
	fmt.Fprintf(io.Out, "%15s: %s\n", "Max Per Region", maxPerRegion)

	if len(processGroups) > 1 {
		for _, pg := range processGroups {
			fmt.Printf("\nProcess group %s\n", pg.Name)
			fmt.Fprintf(io.Out, "%15s: %s\n", "VM Size", pg.VMSize.Name)
			fmt.Fprintf(io.Out, "%15s: %s\n", "VM Memory", formatMemory(*pg.VMSize))
			fmt.Fprintf(io.Out, "%15s: %s\n", "Max Per Region", strconv.Itoa(pg.MaxPerRegion))
		}
	}
}
