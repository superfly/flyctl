package checks

import (
	"context"
	"fmt"
	"sort"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/format"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func runAppCheckList(ctx context.Context) error {
	appName := appconfig.NameFromContext(ctx)
	out := iostreams.FromContext(ctx).Out
	nameFilter := flag.GetString(ctx, "check-name")

	flapsClient, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		AppName: appName,
	})
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

	if config.FromContext(ctx).JSONOutput {
		checks := map[string][]fly.MachineCheckStatus{}
		for _, machine := range machines {
			checks[machine.ID] = make([]fly.MachineCheckStatus, len(machine.Checks))
			for i, check := range machine.Checks {
				checks[machine.ID][i] = *check
			}
		}
		return render.JSON(out, checks)
	}

	fmt.Fprintf(out, "Health Checks for %s\n", appName)
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
			table.Append([]string{check.Name, string(check.Status), machine.ID, format.RelativeTime(*check.UpdatedAt), check.Output})
		}
	}
	table.Render()

	return nil
}
