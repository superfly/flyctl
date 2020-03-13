package client

import (
	"errors"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

var ErrNoAuthToken = errors.New("No api access token available. Please login")

func NewClient() *Client {
	client := &Client{}

	client.InitApi()

	return client
}

type Client struct {
	api    *api.Client
}

func (c *Client) API() *api.Client {
	return c.api
}

func (c *Client) Authenticated() bool {
	return c.api != nil
}

func (c *Client) InitApi() bool {
	if apiToken := viper.GetString(flyctl.ConfigAPIToken); apiToken != "" {
		apiClient := api.NewClient(apiToken, flyctl.Version)
		c.api = apiClient
	}
	return c.Authenticated()
}
