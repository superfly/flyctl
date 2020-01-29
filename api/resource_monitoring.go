package api

func (c *Client) GetAppStatus(appName string, showCompleted bool) (*App, error) {
	query := `
		query($appName: String!, $showCompleted: Boolean!) {
			app(name: $appName) {
				id
				name
				deployed
				status
				hostname
				version
				appUrl
				organization {
					slug
				}
				deploymentStatus {
					id
					status
					version
					description
					placedCount
					promoted
					desiredCount
					healthyCount
					unhealthyCount
				}
				allocations(showCompleted: $showCompleted) {
					id
					idShort
					version
					latestVersion
					status
					desiredStatus
					totalCheckCount
					passingCheckCount
					warningCheckCount
					criticalCheckCount
					createdAt
					updatedAt
					canary
					region
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)
	req.Var("showCompleted", showCompleted)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}
