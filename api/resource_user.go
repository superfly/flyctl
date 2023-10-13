package api

import (
	"context"
)

func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	query := `
		query {
				viewer {
					... on User {
						email
						enablePaidHobby
					}
				}
		}
	`

	req := c.NewRequest(query)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.Viewer, nil
}
