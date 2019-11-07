package api

import "strings"

func (client *Client) GetDatabases() ([]Database, error) {
	q := `
		{
			databases {
				nodes {
					id
					backendId
					key
					name
					engine
					organization {
						id
						name
						slug
					}
					createdAt
				}
			}
		}
	`

	req := client.NewRequest(q)

	data, err := client.Run(req)
	if err != nil {
		return []Database{}, err
	}

	return data.Databases.Nodes, nil
}

func (client *Client) GetDatabase(key string) (*Database, error) {
	q := `
		query($key: String!) {
			database(key: $key) {
				id
				backendId
				key
				name
				engine
				vmUrl
				publicUrl
				organization {
					id
					name
					slug
				}
				createdAt
			}
		}
	`

	req := client.NewRequest(q)

	req.Var("key", key)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.Database, nil
}

func (c *Client) CreateDatabase(orgID string, name string, engine string) (*Database, error) {
	query := `
		mutation($input: CreateDatabaseInput!) {
			createDatabase(input: $input) {
				database {
					id
					backendId
					name
					engine
					vmUrl
					publicUrl
					organization {
						id
						name
						slug
					}
					createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", map[string]string{
		"organizationId": orgID,
		"engine":         strings.ToUpper(engine),
		"name":           name,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateDatabase.Database, nil
}

func (c *Client) DestroyDatabase(databaseId string) (*Organization, error) {
	query := `
		mutation($databaseId: ID!) {
			destroyDatabase(databaseId: $databaseId) {
				organization {
					id
					slug
					name
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("databaseId", databaseId)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.DestroyDatabase.Organization, nil
}
