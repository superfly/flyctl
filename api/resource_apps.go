package api

func (client *Client) GetApps() ([]App, error) {
	query := `
		query {
			apps(type: "container", first: 200) {
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
					status
				}
			}
		}
		`

	req := client.NewRequest(query)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return data.Apps.Nodes, nil
}

func (client *Client) GetAppID(appName string) (string, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				id
			}
		}
	`

	req := client.NewRequest(query)
	req.Var("appName", appName)

	data, err := client.Run(req)
	if err != nil {
		return "", err
	}

	return data.App.ID, nil
}

func (client *Client) GetApp(appName string) (*App, error) {
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
				services {
					description
					protocol
					internalPort
					ports {
						port
						handlers
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

	req := client.NewRequest(query)
	req.Var("appName", appName)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.App, nil
}

func (client *Client) CreateApp(name string, orgId string) (*App, error) {
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

	req := client.NewRequest(query)

	req.Var("input", CreateAppInput{
		Name:           name,
		Runtime:        "FIRECRACKER",
		OrganizationID: orgId,
	})

	data, err := client.Run(req)
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

// PauseApp - Send GQL mutation to pause app
func (client *Client) PauseApp(appName string) (*App, error) {
	query := `
	mutation ($input: PauseAppInput!) {
		pauseApp(input: $input) {
		  app{
			id
			name
			status
			version
			hostname
		  }
		}
	  }
	`

	req := client.NewRequest(query)

	req.Var("input", map[string]string{
		"appId": appName,
	})

	data, err := client.Run(req)
	return &data.PauseApp.App, err
}

// ResumeApp - Send GQL mutation to pause app
func (client *Client) ResumeApp(appName string) (*App, error) {
	query := `
	mutation ($input: ResumeAppInput!) {
		resumeApp(input: $input) {
		  app{
			id
			name
			status
			version
			hostname
		  }
		}
	  }
	`

	req := client.NewRequest(query)

	req.Var("input", map[string]string{
		"appId": appName,
	})

	data, err := client.Run(req)
	return &data.ResumeApp.App, err
}

// RestartApp - Send GQL mutation to restart app
func (client *Client) RestartApp(appName string) (*App, error) {
	query := `
		mutation ($input: RestartAppInput!) {
			restartApp(input: $input) {
				app{
					id
					name
				}
			}
		}
	`

	req := client.NewRequest(query)

	req.Var("input", map[string]string{
		"appId": appName,
	})

	data, err := client.Run(req)
	return &data.RestartApp.App, err
}
