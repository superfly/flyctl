package api

func (c *Client) GetApps() ([]App, error) {
	query := `
		query {
			apps(type: "container") {
				nodes {
					id
					name
					deployed
					hostname
					organization {
						slug
					}
					currentRelease {
						createdAt
					}
				}
			}
		}
		`

	req := c.NewRequest(query)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Apps.Nodes, nil
}

func (c *Client) GetApp(appName string) (*App, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				id
				name
				hostname
				deployed
				status
				version
				appUrl
				organization {
					slug
				}
				tasks {
					id
					name
					services {
						protocol
						softConcurrency
						hardConcurrency
						ports {
							port
							handlers
						}
						internalPort
					}
				}
				ipAddresses {
					nodes {
						id
						address
						type
						createdAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}

func (c *Client) CreateApp(name string, orgId string) (*App, error) {
	query := `
		mutation($input: CreateAppInput!) {
			createApp(input: $input) {
				app {
					id
					name
					organization {
						slug
					}
					config {
						definition
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", CreateAppInput{
		Name:           name,
		Runtime:        "FIRECRACKER",
		OrganizationID: orgId,
	})

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateApp.App, nil
}

func (client *Client) DeleteApp(appName string) error {
	query := `
			mutation($appId: ID!) {
				deleteApp(appId: $appId) {
					organization {
						id
					}
				}
			}
		`

	req := client.NewRequest(query)

	req.Var("appId", appName)

	_, err := client.Run(req)
	return err
}

func (client *Client) MoveApp(appName string, orgID string) (*App, error) {
	query := `
		mutation ($input: MoveAppInput!) {
			moveApp(input: $input) {
				app {
					id
				}
			}
		}
	`

	req := client.NewRequest(query)

	req.Var("input", map[string]string{
		"appId":          appName,
		"organizationId": orgID,
	})

	data, err := client.Run(req)
	return &data.App, err
}
