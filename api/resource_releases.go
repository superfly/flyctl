package api

import "context"

func (c *Client) GetAppReleasesMachines(ctx context.Context, appName, status string, limit int) ([]Release, error) {
	query := `
		query($appName: String!, $limit: Int!) {
			app(name: $appName) {
				releases: releasesUnprocessed(first: $limit) {
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
	ctx = ctxWithAction(ctx, "get_app_releases_machines")
	req.Var("appName", appName)
	req.Var("limit", limit)
	if status != "" {
		req.Var("status", status)
	}

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Releases.Nodes, nil
}

func (c *Client) GetAppCurrentReleaseMachines(ctx context.Context, appName string) (*Release, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				currentRelease: currentReleaseUnprocessed {
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
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)
	ctx = ctxWithAction(ctx, "get_app_current_release_machines")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.CurrentRelease, nil
}
