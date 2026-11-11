package incidents

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/incidents/hosts"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/incidents"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func New() *cobra.Command {
	const (
		short = "Show incidents"
		long  = `Check for problems that may affect your apps. ` + "`fly incidents list`" + ` shows
unresolved incidents from the Fly.io status page, and
` + "`fly incidents hosts list`" + ` shows issues on the hosts that run an app's
Machines.`
	)
	cmd := command.New("incidents", short, long, nil)
	cmd.AddCommand(
		newIncidentsList(),
		hosts.New(),
	)

	return cmd
}

func newIncidentsList() *cobra.Command {
	const (
		short = "List active Fly.io platform incidents"
		long  = `List unresolved incidents posted on the Fly.io status page, with each one's
status, affected components, and start and update times. The list is
platform-wide: --app and --org don't filter it. To open the status page
itself, run ` + "`fly platform status`" + `.`
	)
	cmd := command.New("list", short, long, runIncidentsList,
		command.RequireSession,
		command.LoadAppNameIfPresent,
	)
	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		flag.JSONOutput(),
		flag.Org(),
	)
	cmd.Args = cobra.NoArgs

	return cmd
}

func runIncidentsList(ctx context.Context) error {
	out := iostreams.FromContext(ctx).Out

	statuspageIncidents, err := incidents.StatuspageIncidentsRequest(ctx)
	if err != nil {
		return err
	}

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(out, statuspageIncidents)
	}

	incidentCount := len(statuspageIncidents.Incidents)
	if incidentCount > 0 {
		fmt.Fprintf(out, "Incidents count: %d\n\n", incidentCount)
		table := helpers.MakeSimpleTable(out, []string{"Id", "Name", "Status", "Components", "Started At", "Last Updated"})
		for _, incident := range statuspageIncidents.Incidents {
			var components []string
			for _, component := range incident.Components {
				components = append(components, component.Name)
			}
			table.Append(incident.ID, incident.Name, incident.Status, strings.Join(components, ", "), incident.StartedAt, incident.UpdatedAt) //nolint:errcheck
		}
		table.Render() //nolint:errcheck
	} else {
		fmt.Fprintf(out, "There are no active incidents\n")
	}

	return nil
}
