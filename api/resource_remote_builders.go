package api

import "context"

func (client *Client) EnsureRemoteBuilder(ctx context.Context, orgID string) (*GqlMachine, *AppCompact, error) {
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
						id
						slug
					}
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
		return nil, nil, err
	}

	return data.EnsureMachineRemoteBuilder.Machine, data.EnsureMachineRemoteBuilder.App, nil
}
