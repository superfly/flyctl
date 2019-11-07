package api

func (client *Client) DeployImage(input DeployImageInput) (*Release, error) {
	query := `
			mutation($input: DeployImageInput!) {
				deployImage(input: $input) {
					release {
						id
						version
						reason
						description
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

	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.DeployImage.Release, nil
}
