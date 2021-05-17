package api

func (client *Client) EnsureRemoteBuilderForApp(appName string) (string, *App, error) {
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
		AppName: StringPointer(appName),
	})

	data, err := client.Run(req)
	if err != nil {
		return "", nil, err
	}

	return data.EnsureRemoteBuilder.URL, data.EnsureRemoteBuilder.App, nil
}

func (client *Client) EnsureRemoteBuilderForOrg(orgID string) (string, *App, error) {
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

	data, err := client.Run(req)
	if err != nil {
		return "", nil, err
	}

	return data.EnsureRemoteBuilder.URL, data.EnsureRemoteBuilder.App, nil
}
