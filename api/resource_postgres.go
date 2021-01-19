package api

func (client *Client) CreatePostgresCluster(organizationID string, name string) (*TemplateDeployment, error) {
	query := `
		mutation($input: CreatePostgresClusterInput!) {
			createPostgresCluster(input: $input) {
				templateDeployment {
					id
					status
					apps {
						nodes {
							name
							state
							status
						}
					}
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", map[string]string{
		"organizationId": organizationID,
		"name":           name,
	})

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreatePostgresCluster.TemplateDeployment, nil
}

func (client *Client) GetTemplateDeployment(id string) (*TemplateDeployment, error) {
	query := `
		query($id: ID!) {
			templateDeploymentNode: node(id: $id) {
				... on TemplateDeployment {
					id
					status
					apps {
						nodes {
							name
							state
							status
						}
					}
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("id", id)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return data.TemplateDeploymentNode, nil
}

func (client *Client) AttachPostgresCluster(input AttachPostgresClusterInput) (*App, *App, error) {
	query := `
		mutation($input: AttachPostgresClusterInput!) {
			attachPostgresCluster(input: $input) {
				app {
					name
				}
				postgresClusterApp {
					name
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, nil, err
	}

	return &data.AttachPostgresCluster.App, &data.AttachPostgresCluster.PostgresClusterApp, nil
}

func (client *Client) DetachPostgresCluster(postgresAppName string, appName string) error {
	query := `
		mutation($input: DetachPostgresClusterInput!) {
			detachPostgresCluster(input: $input) {
				clientMutationId
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", map[string]string{
		"postgresClusterAppId": postgresAppName,
		"appId":                appName,
	})

	_, err := client.Run(req)
	return err
}
