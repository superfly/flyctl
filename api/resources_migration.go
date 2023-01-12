package api

import (
	"context"
)

func (c *Client) MigrateNomadToMachines(ctx context.Context, input NomadToMachinesMigrationInput) (*App, error) {
	query := `
		mutation($input: NomadToMachinesMigrationInput!) {
			nomadToMachinesMigration(input: $input) {
				app {
					name
				}
			}
		}
	`
	req := c.NewRequest(query)
	req.Var("input", input)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}
