package api

import "context"

func (c *Client) CreateDoctorUrl(ctx context.Context) (putUrl string, err error) {
	query := `
		mutation {
			createDoctorUrl {
				putUrl
			}
		}
	`

	req := c.NewRequest(query)

	data, err := c.RunWithContext(ctx, req)
	if err != nil {
		return "", err
	}

	return data.CreateDoctorUrl.PutUrl, nil
}
