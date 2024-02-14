package api

import "context"

func (c *Client) CanPerformBluegreenDeployment(ctx context.Context, appName string) (bool, error) {
	query := `
		query ($appName: String!) {
			canPerformBluegreenDeployment(name: $appName)
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)
	ctx = ctxWithAction(ctx, "can_perform_bluegreen_deployment")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return false, err
	}

	return data.CanPerformBluegreenDeployment, nil
}
