package api

import "context"

func (client *Client) EnsureRemoteBuilder(ctx context.Context, orgID string, appName string) (*Machine, *App, error) {
	query := `
		mutation($input: EnsureMachineRemoteBuilderInput!) {
			ensureMachineRemoteBuilder(input: $input) {
				machine {
					id
					state
					ips {
						nodes {
							family
							kind
							ip
						}
					}
				},
				app {
					name
					organization {
						slug
					}
				}
			}
		}
	`

	req := client.NewRequest(query)

	if orgID != "" {
		req.Var("input", EnsureRemoteBuilderInput{
			OrganizationID: StringPointer(orgID),
		})
	} else {
		req.Var("input", EnsureRemoteBuilderInput{
			AppName: StringPointer(appName),
		})

	}

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	return data.EnsureMachineRemoteBuilder.Machine, data.EnsureMachineRemoteBuilder.App, nil
}
