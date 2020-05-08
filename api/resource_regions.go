package api

func (c *Client) ConfigureRegions(input ConfigureRegionsInput) ([]Region, error) {
	query := `
		mutation ($input: ConfigureRegionsInput!) {
			configureRegions(input: $input) {
				regions {
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
		return nil, err
	}

	return data.ConfigureRegions.Regions, nil
}

func (c *Client) ListAppRegions(appName string) ([]Region, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				regions {
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
		return nil, err
	}

	return *data.App.Regions, nil
}
