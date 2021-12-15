package api

import "context"

func (client *Client) RestartAllocation(ctx context.Context, appName string, allocId string) error {
	query := `
		mutation($input: RestartAllocationInput!) {
			restartAllocation(input: $input) {
				app {
					name
				}
				allocation {
					id
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", map[string]string{
		"appId":   appName,
		"allocId": allocId,
	})

	_, err := client.RunWithContext(ctx, req)
	return err
}

func (client *Client) StopAllocation(ctx context.Context, appName string, allocId string) error {
	query := `
		mutation($input: StopAllocationInput!) {
			stopAllocation(input: $input) {
				app {
					name
				}
				allocation {
					id
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", map[string]string{
		"appId":   appName,
		"allocId": allocId,
	})

	_, err := client.RunWithContext(ctx, req)
	return err
}
