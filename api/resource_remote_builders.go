package api

func (client *Client) EnsureRemoteBuilder(appName string) (string, *Release, error) {
	query := `
		mutation($input: EnsureRemoteBuilderInput!) {
			ensureRemoteBuilder(input: $input) {
				url,
				release {
					id
					version
					reason
					description
					deploymentStrategy
					user {
						id
						email
						name
					}
					createdAt
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

	return data.EnsureRemoteBuilder.URL, &data.EnsureRemoteBuilder.Release, nil
}
