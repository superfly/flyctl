package api

import "context"

func (c *Client) GetAppChanges(ctx context.Context, appName string) ([]AppChange, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				changes {
					nodes {
						id
						description
						status
						actor {
							type: __typename
						}
						user {
							id
							email
						}
						createdAt
						updatedAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Changes.Nodes, nil
}
