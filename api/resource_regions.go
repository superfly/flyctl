package api

import "context"

func (c *Client) ConfigureRegions(ctx context.Context, input ConfigureRegionsInput) ([]Region, []Region, error) {
	query := `
		mutation ($input: ConfigureRegionsInput!) {
			configureRegions(input: $input) {
				regions {
					code
					name
				}
				backupRegions{
					code
					name
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", input)
	req.Var("action", "configure_regions")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	return data.ConfigureRegions.Regions, data.ConfigureRegions.BackupRegions, nil
}

func (c *Client) ListAppRegions(ctx context.Context, appName string) ([]Region, []Region, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				regions{
					code
					name
				}
				backupRegions{
					code
					name
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("action", "list_app_regions")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	return *data.App.Regions, *data.App.BackupRegions, nil
}

func (c *Client) GetNearestRegion(ctx context.Context) (*Region, error) {
	req := c.NewRequest(`
		query {
			nearestRegion {
				code
				name
				gatewayAvailable
			}
		}
`)

	req.Var("action", "get_nearest_regions")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.NearestRegion, nil
}
