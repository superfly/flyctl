package client

import (
	"errors"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/logger"
)

var ErrNoAuthToken = errors.New("No access token available. Please login with 'flyctl auth login'")

func New() *Client {
	client := &Client{
		IO: iostreams.System(),
	}

	client.InitApi()

	return client
}

type Client struct {
	IO *iostreams.IOStreams // TODO: remove

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
		apiClient := NewClient(apiToken)
		c.api = apiClient
	}
	return c.Authenticated()
}

func FromToken(token string) *Client {
	var ac *api.Client
	if token != "" {
		ac = NewClient(token)
	}

	return &Client{
		api: ac,
	}
}

func NewClient(token string) *api.Client {
	return api.NewClient(token, buildinfo.Name(), buildinfo.Version().String(), logger.FromEnv(iostreams.System().ErrOut))
}
