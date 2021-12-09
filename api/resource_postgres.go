package api

import (
	"context"
)

func (client *Client) CreatePostgresCluster(ctx context.Context, input CreatePostgresClusterInput) (*CreatePostgresClusterPayload, error) {
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

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.CreatePostgresCluster, nil
}

func (client *Client) GetTemplateDeployment(ctx context.Context, id string) (*TemplateDeployment, error) {
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

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.TemplateDeploymentNode, nil
}

func (client *Client) AttachPostgresCluster(ctx context.Context, input AttachPostgresClusterInput) (*AttachPostgresClusterPayload, error) {
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

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.AttachPostgresCluster, nil
}

func (client *Client) DetachPostgresCluster(ctx context.Context, input DetachPostgresClusterInput) error {
	query := `
		mutation($input: DetachPostgresClusterInput!) {
			detachPostgresCluster(input: $input) {
				clientMutationId
			}
		}
		`

	req := client.NewRequest(query)
	req.Var("input", input)

	_, err := client.RunWithContext(ctx, req)
	return err
}

func (client *Client) ListPostgresDatabases(ctx context.Context, appName string) ([]PostgresClusterDatabase, error) {
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

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return *data.App.PostgresAppRole.Databases, nil
}

func (client *Client) ListPostgresClusterAttachments(ctx context.Context, appName, postgresAppName string) ([]*PostgresClusterAttachment, error) {
	query := `
		query($appName: String!, $postgresAppName: String!) {
			postgresAttachments(appName: $appName, postgresAppName: $postgresAppName) {
				nodes {
					id
					databaseName
					databaseUser
					environmentVariableName
				}
		  }
		}
		`

	req := client.NewRequest(query)
	req.Var("appName", appName)
	req.Var("postgresAppName", postgresAppName)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.PostgresAttachments.Nodes, nil
}

func (client *Client) ListPostgresUsers(ctx context.Context, appName string) ([]PostgresClusterUser, error) {
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

	data, err := client.RunWithContext(ctx, req)
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
