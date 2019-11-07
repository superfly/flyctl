package api

func (c *Client) GetCurrentUser() (*User, error) {
	query := `
		query {
			currentUser {
				email
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.Run(req)
	if err != nil {
		return nil, err
	}

	return &data.CurrentUser, nil
}
