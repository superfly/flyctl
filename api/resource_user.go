package api

import (
	"context"
)

func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	query := `
		query {
				viewer {
					... on User {
						id
						email
						enablePaidHobby
					}
					... on Macaroon {
						email
					}
				}
		}
	`

	req := c.NewRequest(query)
	ctx = ctxWithAction(ctx, "get_current_user")

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.Viewer, nil
}
