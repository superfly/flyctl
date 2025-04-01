package uiex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/httptracing"
	"github.com/superfly/flyctl/internal/logger"
)

type Client struct {
	baseUrl    *url.URL
	tokens     *tokens.Tokens
	httpClient *http.Client
	userAgent  string
}

type NewClientOpts struct {
	// optional, sent with requests
	UserAgent string

	// URL used when connecting via usermode wireguard.
	BaseURL *url.URL

	Tokens *tokens.Tokens

	// optional:
	Logger fly.Logger

	// optional, used to construct the underlying HTTP client
	Transport http.RoundTripper
}

func NewWithOptions(ctx context.Context, opts NewClientOpts) (*Client, error) {
	var err error
	uiexBaseURL := os.Getenv("FLY_UIEX_BASE_URL")

	if uiexBaseURL == "" {
		uiexBaseURL = "https://api.fly.io"
	}
	uiexUrl, err := url.Parse(uiexBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid FLY_UIEX_BASE_URL '%s' with error: %w", uiexBaseURL, err)
	}

	httpClient, err := fly.NewHTTPClient(logger.MaybeFromContext(ctx), httptracing.NewTransport(http.DefaultTransport))
	if err != nil {
		return nil, fmt.Errorf("uiex: can't setup HTTP client to %s: %w", uiexUrl.String(), err)
	}

	userAgent := "flyctl"
	if opts.UserAgent != "" {
		userAgent = opts.UserAgent
	}

	return &Client{
		baseUrl:    uiexUrl,
		tokens:     opts.Tokens,
		httpClient: httpClient,
		userAgent:  userAgent,
	}, nil
}
