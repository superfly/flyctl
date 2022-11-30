package api

import "context"

func (c *Client) GetAllocations(ctx context.Context, appName string, showCompleted bool) ([]*AllocationStatus, error) {
	query := `
		query($appName: String!, $showCompleted: Boolean!) {
			appstatus:app(name: $appName) {
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
					restarts
					healthy
					privateIP
					taskName
					checks {
						status
						output
						name
					}
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)
	req.Var("showCompleted", showCompleted)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.AppStatus.Allocations, nil
}
