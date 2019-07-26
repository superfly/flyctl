package api

import (
	"context"
	"fmt"

	"github.com/machinebox/graphql"
)

type Client struct {
	client      *graphql.Client
	accessToken string
}

func NewClient(baseUrl, accessToken string) *Client {
	client := graphql.NewClient(fmt.Sprintf("%s/api/v2/graphql", baseUrl))
	return &Client{client, accessToken}
}

func (c *Client) Apps(ctx context.Context) (Apps, error) {
	req := graphql.NewRequest(`
query {
		apps(type: "container") {
			nodes {
				id
				name
				runtime
			}
		}
}
		`)
	var resp Apps
	err := c.RunWithContext(ctx, req, &resp)
	return resp, err
}

func (c *Client) Run(req *graphql.Request, resp interface{}) error {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	return c.client.Run(context.Background(), req, resp)
}

func (c *Client) RunWithContext(ctx context.Context, req *graphql.Request, resp interface{}) error {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	return c.client.Run(ctx, req, resp)
}
