package api

func (c *Client) ConfigureRegions(input ConfigureRegionsInput) ([]Region, []Region, error) {
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

	data, err := c.Run(req)
	if err != nil {
		return nil, nil, err
	}

	return data.ConfigureRegions.Regions, data.ConfigureRegions.BackupRegions, nil
}

func (c *Client) ListAppRegions(appName string) ([]Region, []Region, error) {
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

	data, err := c.Run(req)
	if err != nil {
		return nil, nil, err
	}

	return *data.App.Regions, *data.App.BackupRegions, nil
}
