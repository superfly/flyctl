package api

import "context"

func (c *Client) GetAppStatus(ctx context.Context, appName string, showCompleted bool) (*AppStatus, error) {
	query := `
		query($appName: String!, $showCompleted: Boolean!) {
			appstatus:app(name: $appName) {
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

	return &data.AppStatus, nil
}

func (c *Client) GetAllocationStatus(ctx context.Context, appName string, allocID string, logLimit int) (*AllocationStatus, error) {
	query := `
		query($appName: String!, $allocId: String!, $logLimit: Int!) {
			app(name: $appName) {
				allocation(id: $allocId) {
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
					privateIP
					checks {
						status
						output
						name
						serviceName
					}
					events {
						timestamp
						type
						message
					}
					recentLogs(limit: $logLimit) {
						id
						level
						timestamp
						message
					}
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)
	req.Var("allocId", allocID)
	req.Var("logLimit", logLimit)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Allocation, nil
}
