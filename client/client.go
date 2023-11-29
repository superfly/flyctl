package client

import (
	"errors"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/tokens"
	"github.com/superfly/flyctl/iostreams"
)

var ErrNoAuthToken = errors.New("No access token available. Please login with 'flyctl auth login'")

type Client struct {
	IO *iostreams.IOStreams // TODO: remove

	api *api.Client
}

func (c *Client) API() *api.Client {
	return c.api
}

func (c *Client) Authenticated() bool {
	return c.api.Authenticated()
}

func FromTokens(toks *tokens.Tokens) *Client {
	return &Client{api: api.NewClientFromOptions(api.ClientOptions{
		Name:    buildinfo.Name(),
		Version: buildinfo.Version().String(),
		Logger:  logger.FromEnv(iostreams.System().ErrOut).AndLogToFile(),
		Tokens:  toks,
	})}
}

// deprecated
func FromToken(token string) *Client {
	return &Client{
		api: NewClient(token),
	}
}

// deprecated
func NewClient(token string) *api.Client {
	return api.NewClient(
		token,
		buildinfo.Name(),
		buildinfo.Version().String(),
		logger.FromEnv(iostreams.System().ErrOut).AndLogToFile(),
	)
}

type NewClientOpts struct {
	Token         string
	ClientName    string
	ClientVersion string
	Logger        api.Logger
}

// non-flyctl libraries use this when needing to specify logger, client name, and client version
func NewClientWithOptions(opts *NewClientOpts) *Client {
	var log api.Logger
	if opts.Logger != nil {
		log = opts.Logger
	} else {
		log = logger.FromEnv(iostreams.System().ErrOut)
	}
	clientName := buildinfo.Name()
	if opts.ClientName != "" {
		clientName = opts.ClientName
	}
	clientVersion := buildinfo.Version().String()
	if opts.ClientVersion != "" {
		clientVersion = opts.ClientVersion
	}
	return &Client{
		api: api.NewClient(opts.Token, clientName, clientVersion, log),
	}
}
