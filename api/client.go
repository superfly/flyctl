package api

import (
	"context"
	"errors"
	"fmt"

	"github.com/machinebox/graphql"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
)

type Client struct {
	client      *graphql.Client
	accessToken string
}

func NewClient() (*Client, error) {
	accessToken := viper.GetString(flyctl.ConfigAPIAccessToken)
	if accessToken == "" {
		return nil, errors.New("No api access token available. Please login")
	}

	client := graphql.NewClient(fmt.Sprintf("%s/api/v2/graphql", viper.GetString(flyctl.ConfigAPIBaseURL)))
	return &Client{client, accessToken}, nil
}

func (c *Client) NewRequest(q string) *graphql.Request {
	return graphql.NewRequest(q)
}

func (c *Client) Run(req *graphql.Request) (Query, error) {
	return c.RunWithContext(context.Background(), req)
}

func (c *Client) RunWithContext(ctx context.Context, req *graphql.Request) (Query, error) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.accessToken))
	var resp Query
	err := c.client.Run(ctx, req, &resp)
	return resp, err

}
