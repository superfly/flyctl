package api

import "context"

func (client *Client) EnsureMachineRemoteBuilderForApp(ctx context.Context, appName string) (*Machine, *App, error) {
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

	req.Var("input", EnsureRemoteBuilderInput{
		AppName: StringPointer(appName),
	})

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, nil, err
	}

	return data.EnsureMachineRemoteBuilder.Machine, data.EnsureMachineRemoteBuilder.App, nil
}

func (client *Client) EnsureRemoteBuilderForOrg(ctx context.Context, orgID string) (string, *App, error) {
	query := `
		mutation($input: EnsureRemoteBuilderInput!) {
			ensureRemoteBuilder(input: $input) {
				url,
				app {
					name
				}
			}
		}
	`

	req := client.NewRequest(query)

	req.Var("input", EnsureRemoteBuilderInput{
		OrganizationID: StringPointer(orgID),
	})

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return "", nil, err
	}

	return data.EnsureRemoteBuilder.URL, data.EnsureRemoteBuilder.App, nil
}
