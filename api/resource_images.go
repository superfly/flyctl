package api

func (client *Client) GetImageInfo(appName string) (*App, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				imageVersionTrackingEnabled
				imageUpgradeAvailable
				imageDetails {
					registry
					repository
					tag
					digest
					version
				}
				latestImageDetails {
					registry
					repository
					tag
					digest
					version
				}
			}
		}
	`
	req := client.NewRequest(query)
	req.Var("appName", appName)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}
