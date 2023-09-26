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
						evaluationId
						createdAt
					}
					releaseCommand {
						id
						command
						evaluationId
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

func (c *Client) GetDeploymentStatus(ctx context.Context, appName string, deploymentID string, evaluationID string) (*DeploymentStatus, error) {
	query := `
		query ($appName: String!, $deploymentId: ID!, $evaluationId: String!) {
			app(name: $appName) {
				deploymentStatus(id: $deploymentId, evaluationId: $evaluationId) {
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
	req.Var("evaluationId", evaluationID)

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

func (c *Client) CanPerformBluegreenDeployment(ctx context.Context, appName string) (bool, error) {
	query := `
		query ($appName: String!) {
			canPerformBluegreenDeployment(name: $appName)
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return false, err
	}

	return data.CanPerformBluegreenDeployment, nil
}
