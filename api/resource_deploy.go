package api

import "context"

func (client *Client) DeployImage(ctx context.Context, input DeployImageInput) (*Release, *ReleaseCommand, error) {
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
					releaseCommand {
						id
						command
					}
				}
			}
		`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	return &data.DeployImage.Release, data.DeployImage.ReleaseCommand, nil
}

func (c *Client) GetDeploymentStatus(ctx context.Context, appName string, deploymentID string) (*DeploymentStatus, error) {
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

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.DeploymentStatus, nil
}

func (c *Client) GetReleaseCommand(ctx context.Context, id string) (*ReleaseCommand, error) {
	query := `
		query ($id: ID!) {
			releaseCommandNode: node(id: $id) {
				id
				... on ReleaseCommand {
					id
					instanceId
					command
					status
					exitCode
					inProgress
					succeeded
					failed
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("id", id)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.ReleaseCommandNode, nil
}
