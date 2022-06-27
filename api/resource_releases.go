package api

import "context"

func (c *Client) GetAppReleases(ctx context.Context, appName string, limit int) ([]Release, error) {
	query := `
		query ($appName: String!, $limit: Int!) {
			app(name: $appName) {
				releases(first: $limit) {
					nodes {
						id
						version
						description
						reason
						status
						imageRef
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

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Releases.Nodes, nil
}

func (c *Client) GetAppRelease(ctx context.Context, appName string, id string) (*Release, error) {
	query := `
		query ($appName: String!, $releaseId: ID!) {
			app(name: $appName) {
				release(id: $releaseId) {
					id
					status
					evaluationId
					createdAt
					deploymentStrategy
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	req.Var("releaseId", id)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Release, nil
}
