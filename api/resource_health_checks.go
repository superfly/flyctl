package api

import "context"

func (client *Client) GetAppHealthChecks(ctx context.Context, appName string, checkName *string, limitOutput *int, compactOutput *bool) ([]CheckState, error) {
	q := `
		query($appName: String!, $checkName: String, $limitOutput: Int, $compactOutput: Boolean) {
			app(name: $appName) {
				healthChecks(name: $checkName) {
					nodes {
						allocation {
							idShort
							region
						}
						name
						status
						serviceName
						output(limit: $limitOutput, compact: $compactOutput)
						type
						updatedAt
					}
				}
			}
		}
	`

	req := client.NewRequest(q)
	req.Var("appName", appName)
	req.Var("checkName", checkName)
	req.Var("limitOutput", limitOutput)
	req.Var("compactOutput", compactOutput)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.HealthChecks.Nodes, nil
}
