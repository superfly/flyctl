package api

func (client *Client) DeployImage(input DeployImageInput) (*Release, error) {
	query := `
			mutation($input: DeployImageInput!) {
				deployImage(input: $input) {
					release {
						id
						version
						reason
						description
						deploymentStrategy
						user {
							id
							email
							name
						}
						createdAt
					}
				}
			}
		`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.DeployImage.Release, nil
}

func (c *Client) GetDeploymentStatus(appName string, deploymentID string) (*DeploymentStatus, error) {
	query := `
		query ($appName: String!, $deploymentId: ID!) {
			app(name: $appName) {
				deploymentStatus(id: $deploymentId) {
					id
					inProgress
					status
					successful
					description
					version
					desiredCount
					placedCount
					healthyCount
					unhealthyCount
					allocations {
						id
						idShort
						status
						region
						desiredStatus
						version
						healthy
            			failed
						canary
						restarts
						checks {
							status
							serviceName
						}
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("deploymentId", deploymentID)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.DeploymentStatus, nil
}
