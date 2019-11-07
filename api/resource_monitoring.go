package api

func (c *Client) GetAppStatus(appName string, showComplete bool) (*App, error) {
	query := `
		query($appName: String!, $showComplete: Boolean!) {
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
				tasks {
					id
					name
					status
					servicesSummary
					allocations(complete: $showComplete) {
						id
						version
						latestVersion
						status
						desiredStatus
						region
						createdAt
					}
				}
				currentRelease {
					version
					stable
					inProgress
					description
					status
					reason
					revertedTo
					deploymentStrategy
					user {
						email
					}
					createdAt
					deployment {
						status
						description
						tasks {
							name
							placed
							healthy
							desired
							canaries
							promoted
							unhealthy
							progressDeadline
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)
	req.Var("showComplete", showComplete)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}
