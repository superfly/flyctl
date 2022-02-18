package api

import "context"

func (c *Client) CreateSignedUrls(ctx context.Context, appId string, filename string) (getUrl string, putUrl string, err error) {
	query := `
		mutation($appId: ID!, $filename: String!) {
			createSignedUrl(appId: $appId, filename: $filename) {
				getUrl
				putUrl
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appId", appId)
	req.Var("filename", filename)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return "", "", err
	}

	return data.CreateSignedUrl.GetUrl, data.CreateSignedUrl.PutUrl, nil
}

func (c *Client) StartSourceBuild(ctx context.Context, input StartSourceBuildInput) (*SourceBuild, error) {
	query := `
		mutation($input: StartBuildInput!) {
			startBuild(input: $input) {
				sourceBuild {
					id
				}
			}
		}
	`

	req := c.NewRequest(query)
	req.Var("input", input)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.StartBuild.SourceBuild, nil
}

func (c *Client) UpdateSourceBuild(ctx context.Context, input UpdateSourceBuildInput) (*SourceBuild, error) {
	query := `
		mutation($input: UpdateBuildInput!) {
			updateBuild(input: $input) {
				sourceBuild {
					id
	  				status
	  				createdBy{
						email
	  				}
	  				logs
	  				createdAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("input", input)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}
	return data.StartBuild.SourceBuild, nil
}

func (c *Client) ListBuilds(ctx context.Context, appName string) ([]SourceBuild, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				sourceBuilds {
					nodes {
						id
						logs
						image
						status
						createdBy {
							id
							name
							email
						}
						createdAt
						updatedAt
					}
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.SourceBuilds.Nodes, nil
}

func (c *Client) GetBuild(ctx context.Context, buildId string) (*Build, error) {
	query := `
		query($id: ID!) {
			build: node(id: $id) {
				id
				__typename
				... on Build {
					inProgress
					status
					logs
					image
					user {
						id
						name
						email
					}
					createdAt
					updatedAt
				}
			}
		}
	`

	req := c.NewRequest(query)

	req.Var("id", buildId)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.Build, nil
}
