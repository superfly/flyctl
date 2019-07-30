package cmd

import (
	"os"

	"github.com/olekukonko/tablewriter"
	"github.com/superfly/flyctl/api"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(appsCmd)
}

var appsCmd = &cobra.Command{
	Use: "apps",
	// Short: "Print the version number of flyctl",
	// Long:  `All software has versions. This is flyctl`,
	RunE: runApps,
}

func runApps(cmd *cobra.Command, args []string) error {
	client, err := api.NewClient()
	if err != nil {
		return err
	}

	query := `
		query {
			apps {
				nodes {
					id
					name
					organization {
						slug
					}
					runtime
				}
			}
		}
	`

	req := client.NewRequest(query)

	data, err := client.Run(req)
	if err != nil {
		return err
	}

	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Name", "Owner", "Runtime"})

	for _, app := range data.Apps.Nodes {
		table.Append([]string{app.Name, app.Organization.Slug, app.Runtime})
	}

	table.Render()

	return nil
}
