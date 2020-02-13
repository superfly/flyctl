package api

func (c *Client) ScaleApp(appID string, regions []ScaleRegionInput) ([]ScaleRegionChange, error) {
	query := `
		mutation ($input: ScaleAppInput!) {
			scaleApp(input: $input) {
				placement {
					region
					count
				}
				delta {
					region
					fromCount
					toCount
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", ScaleAppInput{AppID: appID, Regions: regions})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.ScaleApp.Delta, nil
}
