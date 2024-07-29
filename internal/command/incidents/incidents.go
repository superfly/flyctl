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
		long  = `Show incidents that may be affecting your organization.`
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
		short = "List active incidents."
		long  = `List the active incidents that may be affecting your apps or deployments.`
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
		table.SetRowLine(true)
		for _, incident := range statuspageIncidents.Incidents {
			var components []string
			for _, component := range incident.Components {
				components = append(components, component.Name)
			}
			table.Append([]string{incident.ID, incident.Name, incident.Status, strings.Join(components, ", "), incident.StartedAt, incident.UpdatedAt})
		}
		table.Render()
	} else {
		fmt.Fprintf(out, "There are no active incidents\n")
	}

	return nil
}
