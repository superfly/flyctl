package checks

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func runAppCheckList(ctx context.Context) error {
	web := client.FromContext(ctx).API()
	appName := app.NameFromContext(ctx)

	app, err := web.GetAppCompact(ctx, appName)
	if err != nil {
		return fmt.Errorf("failed to get app: %s", err)
	}

	if app.PlatformVersion == "machines" {
		return runMachinesAppCheckList(ctx, app)
	}
	return runNomadAppCheckList(ctx)
}

func runMachinesAppCheckList(ctx context.Context, app *api.AppCompact) error {
	out := iostreams.FromContext(ctx).Out
	nameFilter := flag.GetString(ctx, "check-name")

	flapsClient, err := flaps.New(ctx, app)
	if err != nil {
		return err
	}

	machines, err := flapsClient.ListActive(ctx)
	if err != nil {
		return err
	}
	sort.Slice(machines, func(i, j int) bool {
		return machines[i].ID < machines[j].ID
	})

	fmt.Fprintf(out, "Health Checks for %s\n", app.Name)
	table := helpers.MakeSimpleTable(out, []string{"Name", "Status", "Machine", "Last Updated", "Output"})
	table.SetRowLine(true)
	for _, machine := range machines {
		sort.Slice(machine.Checks, func(i, j int) bool {
			return machine.Checks[i].Name < machine.Checks[j].Name
		})

		for _, check := range machine.Checks {
			if nameFilter != "" && nameFilter != check.Name {
				continue
			}
			table.Append([]string{check.Name, check.Status, machine.ID, format.RelativeTime(*check.UpdatedAt), check.Output})
		}
	}
	table.Render()

	return nil
}

func runNomadAppCheckList(ctx context.Context) error {
	appName := app.NameFromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	web := client.FromContext(ctx).API()

	var nameFilter *string
	if val := flag.GetString(ctx, "check-name"); val != "" {
		nameFilter = api.StringPointer(val)
	}

	checks, err := web.GetAppHealthChecks(ctx, appName, nameFilter, nil, api.BoolPointer(false))
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, checks)
	}

	fmt.Fprintf(out, "Health Checks for %s\n", appName)
	table := helpers.MakeSimpleTable(out, []string{"Name", "Status", "Allocation", "Region", "Type", "Last Updated", "Output"})
	for _, check := range checks {
		formattedOutput := formatOutput(check.Output)
		table.Append([]string{check.Name, check.Status, check.Allocation.IDShort, check.Allocation.Region, check.Type, format.RelativeTime(check.UpdatedAt), formattedOutput})
	}
	table.Render()

	return nil
}

func formatOutput(output string) string {
	var newstr string
	output = strings.ReplaceAll(output, "\n", "")
	output = strings.ReplaceAll(output, "] ", "]")
	v := strings.Split(output, "[✓]")
	for _, attr := range v {
		newstr += fmt.Sprintf("%s[✓]\n\n", attr)
	}
	return newstr
}
