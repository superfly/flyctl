package api

func (client *Client) CreatePostgresCluster(input CreatePostgresClusterInput) (*CreatePostgresClusterPayload, error) {
	query := `
		mutation($input: CreatePostgresClusterInput!) {
			createPostgresCluster(input: $input) {
				app {
					name
				}
				username
				password
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return data.CreatePostgresCluster, nil
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
							deployed
							currentRelease{
								createdAt
							}
							organization{
								slug
							}
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

func (client *Client) AttachPostgresCluster(input AttachPostgresClusterInput) (*AttachPostgresClusterPayload, error) {
	query := `
		mutation($input: AttachPostgresClusterInput!) {
			attachPostgresCluster(input: $input) {
				app {
					name
				}
				postgresClusterApp {
					name
				}
				environmentVariableName
				connectionString
				environmentVariableName
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return data.AttachPostgresCluster, nil
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

func (client *Client) ListPostgresDatabases(appName string) ([]PostgresClusterDatabase, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				postgresAppRole: role {
					name
					... on PostgresClusterAppRole {
						databases {
							name
							users
						}
					}
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("appName", appName)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return *data.App.PostgresAppRole.Databases, nil
}

func (client *Client) ListPostgresUsers(appName string) ([]PostgresClusterUser, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				postgresAppRole: role {
					name
					... on PostgresClusterAppRole {
						users {
							username
							isSuperuser
							databases
						}
					}
				}
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("appName", appName)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return *data.App.PostgresAppRole.Users, nil
}

// func (client *Client) CreatePostgresDatabase(name string) (*PostgresClusterUser, error) {
// 	query := `
// 		mutation($input: CreatePostgresClusterUserInput!) {
// 			createPostgresClusterUser(input: $input) {
// 				user {
// 					username
// 				}
// 			}
// 		}
// 		`

// 	req := client.NewRequest(query)
// 	req.Var("input", map[string]interface{}{
// 		"username":  username,
// 		"password":  password,
// 		"superuser": superuser,
// 	})

// 	data, err := client.Run(req)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return *data.App.PostgresAppRole.Users, nil
// }

// func (client *Client) CreatePostgresDatabase(database string) (PostgresClusterDatabase, error) {
// 	query := `
// 		mutation($appName: String!) {
// 			app(name: $appName) {
// 				postgresAppRole: role {
// 					name
// 					... on PostgresClusterAppRole {
// 						users {
// 							username
// 							isSuperuser
// 							databases
// 						}
// 					}
// 				}
// 			}
// 		}
// 		`

// 	req := client.NewRequest(query)
// 	req.Var("appName", appName)

// 	data, err := client.Run(req)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return *data.App.PostgresAppRole.Users, nil
// }
