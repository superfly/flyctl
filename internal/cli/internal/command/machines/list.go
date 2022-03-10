package machines

import (
	"context"
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/cli/internal/app"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/client"
)

func newList() *cobra.Command {
	const (
		long = `Lists all running fly machines with identifying ID, image, state, region and name`

		short = "List all running fly machines"

		usage = "list"
	)

	cmd := command.New(usage, short, long, runList,
		command.RequireSession,
	)

	cmd.Args = cobra.NoArgs
	return cmd
}

func runList(ctx context.Context) error {
	var (
		appName = app.NameFromContext(ctx)
		client  = client.FromContext(ctx).API()
	)

	machines, err := client.ListMachines(ctx, appName, "")
	if err != nil {
		return errors.Wrap(err, "could not get list of machines")
	}

	data := [][]string{}

	for _, machine := range machines {

		var ipv6 string

		for _, ip := range machine.IPs.Nodes {
			if ip.Family == "v6" && ip.Kind == "privatenet" {
				ipv6 = ip.IP
			}
		}

		row := []string{
			machine.ID,
			machine.Config.Image,
			machine.CreatedAt.String(),
			machine.State,
			machine.Region,
			machine.Name,
			ipv6,
		}
		if appName == "" {
			row = append(row, machine.App.Name)
		}
		data = append(data, row)
	}

	table := tablewriter.NewWriter(os.Stdout)
	headers := []string{"ID", "Image", "Created", "State", "Region", "Name", "IP Address"}
	if appName == "" {
		headers = append(headers, "App")
	}
	table.SetHeader(headers)
	table.SetAutoWrapText(false)
	table.SetAutoFormatHeaders(true)
	table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetCenterSeparator("")
	table.SetColumnSeparator("")
	table.SetRowSeparator("")
	table.SetHeaderLine(false)
	table.SetBorder(false)
	table.SetTablePadding("\t") // pad with tabs
	table.SetNoWhiteSpace(true)
	table.AppendBulk(data) // Add Bulk Data
	table.Render()

	return nil
}
