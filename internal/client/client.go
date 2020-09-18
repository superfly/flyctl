package client

import (
	"errors"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

var ErrNoAuthToken = errors.New("No access token available. Please login with 'flyctl auth login'")

func NewClient() *Client {
	client := &Client{}

	client.InitApi()

	return client
}

type Client struct {
	api *api.Client
}

func (c *Client) API() *api.Client {
	return c.api
}

func (c *Client) Authenticated() bool {
	return c.api != nil
}

func (c *Client) InitApi() bool {
	apiToken := flyctl.GetAPIToken()
	if apiToken != "" {
		apiClient := api.NewClient(apiToken, flyctl.Version)
		c.api = apiClient
	}
	return c.Authenticated()
}
