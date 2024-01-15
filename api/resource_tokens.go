package api

import (
	"context"
)

func (c *Client) GetAppLimitedAccessTokens(ctx context.Context, appName string) ([]LimitedAccessToken, error) {
	query := `
		query ($appName: String!) {
			app(name: $appName) {
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

	req := c.NewRequest(query)
	req.Var("appName", appName)
	ctx = ctxWithAction(ctx, "get_app_limited_access_tokens")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return data.App.LimitedAccessTokens.Nodes, nil
}

func (c *Client) RevokeLimitedAccessToken(ctx context.Context, id string) error {
	query := `
		mutation($input:DeleteLimitedAccessTokenInput!) {
			deleteLimitedAccessToken(input: $input) {
				token
			}
		}
	`
	req := c.NewRequest(query)

	req.Var("input", map[string]interface{}{
		"id": id,
	})
	ctx = ctxWithAction(ctx, "revoke_limited_access_token")

	_, err := c.RunWithContext(ctx, req)
	if err != nil {
		return err
	}

	return nil
}
