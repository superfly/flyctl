package api

func (c *Client) CreateSignedUrls(appId string, filename string) (getUrl string, putUrl string, err error) {
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

	data, err := c.Run(req)
	if err != nil {
		return "", "", err
	}

	return data.CreateSignedUrl.GetUrl, data.CreateSignedUrl.PutUrl, nil
}

func (c *Client) CreateBuild(appId string, sourceUrl, sourceType string) (*Build, error) {
	query := `
		mutation($appId: ID!, $sourceUrl: String!, $sourceType: UrlSource!) {
			createBuild(appId: $appId, sourceUrl: $sourceUrl, sourceType: $sourceType) {
				build {
					id
					inProgress
					status
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

	req.Var("appId", appId)
	req.Var("sourceUrl", sourceUrl)
	req.Var("sourceType", sourceType)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CreateBuild.Build, nil
}

func (c *Client) ListBuilds(appName string) ([]Build, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				builds {
					nodes {
						id
						inProgress
						status
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
		}
	`

	req := c.NewRequest(query)

	req.Var("appName", appName)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return data.App.Builds.Nodes, nil
}

func (c *Client) GetBuild(buildId string) (*Build, error) {
	query := `
		query($id: ID!) {
			build: node(id: $id) {
				id
				__typename
				... on Build {
					inProgress
					status
					logs
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

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.Build, nil
}
