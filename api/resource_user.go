package api

import "context"

func (c *Client) GetCurrentUser(ctx context.Context) (*User, error) {
	query := `
		query {
			currentUser {
				email
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return nil, err
	}

	return &data.CurrentUser, nil
}
