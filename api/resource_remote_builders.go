package api

func (client *Client) EnsureRemoteBuilder(appName string) (string, *App, error) {
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
		AppName: appName,
	})

	data, err := client.Run(req)
	if err != nil {
		return "", nil, err
	}

	return data.EnsureRemoteBuilder.URL, data.EnsureRemoteBuilder.App, nil
}
