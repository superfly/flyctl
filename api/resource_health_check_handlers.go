package api

import "context"

func (client *Client) GetHealthCheckHandlers(ctx context.Context, organizationSlug string) ([]HealthCheckHandler, error) {
	q := `
		query($slug: String!) {
			organization(slug: $slug) {
				healthCheckHandlers {
					nodes {
						name
						type
					}
				}
			}
		}
	`

	req := client.NewRequest(q)
	req.Var("slug", organizationSlug)
	ctx = ctxWithAction(ctx, "get_healthcheck_and_handlers")

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.Organization.HealthCheckHandlers.Nodes, nil
}

func (client *Client) SetSlackHealthCheckHandler(ctx context.Context, input SetSlackHandlerInput) (*HealthCheckHandler, error) {
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
	ctx = ctxWithAction(ctx, "set_slack_healthcheck_handler")

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.SetSlackHandler.Handler, nil
}

func (client *Client) SetPagerdutyHealthCheckHandler(ctx context.Context, input SetPagerdutyHandlerInput) (*HealthCheckHandler, error) {
	q := `
		mutation($input: SetPagerdutyHandlerInput!) {
			setPagerdutyHandler(input: $input) {
				handler {
					name
					type
				}
			}
		}
	`

	req := client.NewRequest(q)
	req.Var("input", input)
	ctx = ctxWithAction(ctx, "set_pagerduty_healthcheck_handler")

	data, err := client.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.SetPagerdutyHandler.Handler, nil
}

func (client *Client) DeleteHealthCheckHandler(ctx context.Context, orgID string, handlerName string) error {
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
	ctx = ctxWithAction(ctx, "delete_healthcheck_handler")

	_, err := client.RunWithContext(ctx, req)

	return err
}
