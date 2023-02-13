package api

import "context"

func (client *Client) GetApps(ctx context.Context, role *string) ([]App, error) {
	query := `
		query($role: String) {
			apps(type: "container", first: 400, role: $role) {
				nodes {
					id
					name
					deployed
					hostname
					platformVersion
					organization {
						slug
					}
					currentRelease {
						createdAt
						status

					}
					status
				}
			}
		}
		`

	req := client.NewRequest(query)
	if role != nil {
		req.Var("role", *role)
	}

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.Apps.Nodes, nil
}

func (client *Client) GetAppID(ctx context.Context, appName string) (string, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
				id
			}
		}
	`

	req := client.NewRequest(query)
	req.Var("appName", appName)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return "", err
	}

	return data.App.ID, nil
}

func (client *Client) GetApp(ctx context.Context, appName string) (*App, error) {
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
				platformVersion
				currentRelease {
					evaluationId
					status
					inProgress
					version
				}
				config {
					definition
				}
				organization {
					id
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
				imageDetails {
					repository
					version
				}
				machines{
					nodes {
						id
						name
						config
						state
						region
						createdAt
						app {
							name
						}
						ips {
							nodes {
								family
								kind
								ip
								maskSize
							}
						}
						host {
							id
						}
					}
				}
				postgresAppRole: role {
					name
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

	return &data.App, nil
}

func (client *Client) GetAppCompact(ctx context.Context, appName string) (*AppCompact, error) {
	query := `
		query ($appName: String!) {
			appcompact:app(name: $appName) {
				id
				name
				hostname
				deployed
				status
				appUrl
				platformVersion
				organization {
					id
					slug
				}
				postgresAppRole: role {
					name
				}
				imageDetails {
					repository
					version
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

	return &data.AppCompact, nil
}

func (client *Client) GetAppInfo(ctx context.Context, appName string) (*AppInfo, error) {
	query := `
		query ($appName: String!) {
			appinfo:app(name: $appName) {
				id
				name
				hostname
				deployed
				status
				version
				appUrl
				platformVersion
				organization {
					id
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
						region
						createdAt
					}
				}
				postgresAppRole: role {
					name
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

	return &data.AppInfo, nil
}

func (client *Client) GetAppBasic(ctx context.Context, appName string) (*AppBasic, error) {
	query := `
		query ($appName: String!) {
			appbasic:app(name: $appName) {
				id
				name
				platformVersion
				organization {
					id
					slug
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

	return &data.AppBasic, nil
}

func (client *Client) GetAppMonitoring(ctx context.Context, appName string) (*AppMonitoring, error) {
	query := `
		query ($appName: String!) {
			appmonitoring:app(name: $appName) {
				id
				currentRelease {
					evaluationId
					status
					inProgress
					version
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

	return &data.AppMonitoring, nil
}

func (client *Client) GetAppPostgres(ctx context.Context, appName string) (*AppPostgres, error) {
	query := `
		query ($appName: String!) {
			apppostgres:app(name: $appName) {
				id
				name
				organization {
					id
					slug
				}
				imageDetails {
					repository
					version
				}
				postgresAppRole: role {
					name
				}
				platformVersion
			}
		}
	`

	req := client.NewRequest(query)
	req.Var("appName", appName)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.AppPostgres, nil
}

func (client *Client) CreateApp(ctx context.Context, input CreateAppInput) (*App, error) {
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
					regions {
							name
							code
					}
				}
			}
		}
	`

	req := client.NewRequest(query)

	req.Var("input", input)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.CreateApp.App, nil
}

func (client *Client) DeleteApp(ctx context.Context, appName string) error {
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

	_, err := client.RunWithContext(ctx, req)
	return err
}

func (client *Client) MoveApp(ctx context.Context, appName string, orgID string) (*App, error) {
	query := `
		mutation ($input: MoveAppInput!) {
			moveApp(input: $input) {
				app {
					id
					networkId
					organization {
						slug
					}
				}
			}
		}
	`

	req := client.NewRequest(query)

	req.Var("input", map[string]string{
		"appId":          appName,
		"organizationId": orgID,
	})

	data, err := client.RunWithContext(ctx, req)
	return &data.App, err
}

// SuspendApp - Send GQL mutation to suspend app
func (client *Client) SuspendApp(ctx context.Context, appName string) (*App, error) {
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

	data, err := client.RunWithContext(ctx, req)
	return &data.SuspendApp.App, err
}

// ResumeApp - Send GQL mutation to pause app
func (client *Client) ResumeApp(ctx context.Context, appName string) (*AppCompact, error) {
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

	data, err := client.RunWithContext(ctx, req)
	return &data.ResumeApp.App, err
}

// RestartApp - Send GQL mutation to restart app
func (client *Client) RestartApp(ctx context.Context, appName string) (*App, error) {
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

	data, err := client.RunWithContext(ctx, req)
	return &data.RestartApp.App, err
}

func (client *Client) ResolveImageForApp(ctx context.Context, appName, imageRef string) (*Image, error) {
	query := `
		query ($appName: String!, $imageRef: String!) {
			app(name: $appName) {
				id
				image(ref: $imageRef) {
					id
					digest
					ref
					compressedSize
				}
			}
		}
	`

	req := client.NewRequest(query)
	req.Var("appName", appName)
	req.Var("imageRef", imageRef)

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Image, nil
}
