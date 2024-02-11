package api

import (
	"context"
)

func (client *Client) GetApps(ctx context.Context, role *string) ([]App, error) {
	more := true
	apps := []App{}
	var cursor string

	for more {
		var appPage []App
		var err error

		appPage, more, cursor, err = client.getAppsPage(ctx, nil, role, &cursor)
		if err != nil {
			return nil, err
		}
		apps = append(apps, appPage...)
	}

	return apps, nil
}

func (client *Client) GetAppsForOrganization(ctx context.Context, orgID string) ([]App, error) {
	more := true
	apps := []App{}
	var cursor string

	for more {
		var appPage []App
		var err error

		appPage, more, cursor, err = client.getAppsPage(ctx, &orgID, nil, &cursor)
		if err != nil {
			return nil, err
		}
		apps = append(apps, appPage...)
	}

	return apps, nil
}

func (client *Client) getAppsPage(ctx context.Context, orgID *string, role *string, after *string) ([]App, bool, string, error) {
	query := `
		query($org: ID, $role: String, $after: String) {
			apps(type: "container", first: 200, after: $after, organizationId: $org, role: $role) {
				pageInfo {
					hasNextPage
					endCursor
				}
				nodes {
					id
					name
					deployed
					hostname
					platformVersion
					organization {
						slug
						name
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
	ctx = ctxWithAction(ctx, "get_apps_page")
	if orgID != nil {
		req.Var("org", *orgID)
	}
	if role != nil {
		req.Var("role", *role)
	}
	if after != nil {
		req.Var("after", *after)
	}

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, false, "", err
	}

	return data.Apps.Nodes, data.Apps.PageInfo.HasNextPage, data.Apps.PageInfo.EndCursor, nil
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
					paidPlan
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
					registry
					repository
					tag
					digest
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
				limitedAccessTokens {
					nodes {
						id
						name
						expiresAt
					}
				}
			}
		}
	`

	req := client.NewRequest(query)
	req.Var("appName", appName)
	ctx = ctxWithAction(ctx, "get_app")

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
					paidPlan
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
	ctx = ctxWithAction(ctx, "get_app_compact")

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.AppCompact, nil
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
					rawSlug
					paidPlan
				}
			}
		}
	`

	req := client.NewRequest(query)
	req.Var("appName", appName)
	ctx = ctxWithAction(ctx, "get_app_basic")

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.AppBasic, nil
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
	ctx = ctxWithAction(ctx, "create_app")

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
	ctx = ctxWithAction(ctx, "delete_app")

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
	ctx = ctxWithAction(ctx, "move_app")

	data, err := client.RunWithContext(ctx, req)
	return &data.App, err
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
					compressedSize: compressedSizeFull
				}
			}
		}
	`

	req := client.NewRequest(query)
	req.Var("appName", appName)
	req.Var("imageRef", imageRef)
	ctx = ctxWithAction(ctx, "resolve_image")

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.Image, nil
}
