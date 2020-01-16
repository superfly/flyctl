package api

func (c *Client) GetAppReleases(appName string, limit int) ([]Release, error) {
	query := `
		query ($appName: String!, $limit: Int!) {
			app(name: $appName) {
				releases(first: $limit) {
					nodes {
						id
						version
						reason
						description
						reason
						status
						stable
						user {
							id
							email
							name
						}	
						createdAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("limit", limit)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Releases.Nodes, nil
}

func (c *Client) GetAppCurrentRelease(appName string) (*Release, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				currentRelease {
					version
					stable
					inProgress
					description
					status
					reason
					revertedTo
					deploymentStrategy
					createdAt
					user {
						email
					}
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

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.CurrentRelease, nil
}

func (c *Client) GetAppReleaseVersion(appName string, version int) (*Release, error) {
	query := `
		query ($appName: String!, $version: Int!) {
			app(name: $appName) {
				release(version: $version) {
					version
					stable
					inProgress
					description
					status
					reason
					revertedTo
					deploymentStrategy
					createdAt
					user {
						email
					}
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
	req.Var("version", version)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Release, nil
}
