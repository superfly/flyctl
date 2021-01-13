package api

func (client *Client) GetHealthCheckHandlers(organizationSlug string) ([]HealthCheckHandler, error) {
	q := `
		query($slug: String!) {
			organization(slug: $slug) {
				healthCheckHandlers {
					name
					type
				}
			}
		}
	`

	req := client.NewRequest(q)
	req.Var("slug", organizationSlug)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return *data.Organization.HealthCheckHandlers, nil
}

func (client *Client) SetSlackHealthCheckHandler(input SetSlackHandlerInput) (*HealthCheckHandler, error) {
	q := `
		mutation($input: SetSlackHandlerInput!) {
			setSlackHandler(input: $input) {
				handler {
					name
					type
				}
			}
		}
	`

	req := client.NewRequest(q)
	req.Var("input", input)

	data, err := client.Run(req)
	if err != nil {
		return nil, err
	}

	return data.SetSlackHandler.Handler, nil
}

func (client *Client) DeleteHealthCheckHandler(orgID string, handlerName string) error {
	q := `
		mutation($input: DeleteHealthCheckHandlerInput!) {
			deleteHealthCheckHandler(input: $input) {
				clientMutationId
			}
		}
	`

	req := client.NewRequest(q)
	req.Var("input", map[string]string{
		"organizationId": orgID,
		"name":           handlerName,
	})

	_, err := client.Run(req)

	return err
}
