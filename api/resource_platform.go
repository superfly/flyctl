package api

func (c *Client) PlatformRegions() ([]Region, error) {
	query := `
		query {
			platform {
				regions {
					name
					code
					latitude
					longitude
				}
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Platform.Regions, nil
}
