package api

func (c *Client) GetAppServices(appName string) ([]Service, error) {
	query := `
		query($appName: String!) {
			app(name: $appName) {
				services {
					protocol
					softConcurrency
					hardConcurrency
					ports {
						port
						handlers
					}
					internalPort
					description
					checks {
						interval
						timeout
						type
						httpPath
						httpMethod
						httpHeaders {
							name
							value
						}
						httpProtocol
						httpTlsSkipVerify
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

	return data.App.Services, nil
}
