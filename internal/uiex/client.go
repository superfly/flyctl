package uiex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/superfly/client-signals/go"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/httptracing"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/requestauth"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
)

const defaultHTTPTimeout = 30 * time.Second

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

	// optional; if non-nil, attaches the Fly-Client-* headers/UA suffix
	// derived from these signals
	ClientSignals *clientsignals.Signals

	// optional; bounds each HTTP request made by the client.
	// Defaults to 30s when zero or negative.
	Timeout time.Duration
}

func NewWithOptions(ctx context.Context, opts NewClientOpts) (*Client, error) {
	var err error

	baseUrl := opts.BaseURL
	if baseUrl == nil {
		uiexBaseURL := os.Getenv("FLY_UIEX_BASE_URL")

		if uiexBaseURL == "" {
			uiexBaseURL = "https://api.fly.io"
		}
		uiexUrl, err := url.Parse(uiexBaseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid FLY_UIEX_BASE_URL '%s' with error: %w", uiexBaseURL, err)
		}

		baseUrl = uiexUrl
	}

	// Clients are constructed before deploy initializes the global propagator.
	var transport = httptracing.NewTransport(otelhttp.NewTransport(requestauth.Transport(http.DefaultTransport),
		otelhttp.WithPropagators(propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		)),
	))
	if opts.ClientSignals != nil {
		transport = opts.ClientSignals.WrapTransport(transport)
	}

	httpClient, err := fly.NewHTTPClient(logger.MaybeFromContext(ctx), transport)
	if err != nil {
		return nil, fmt.Errorf("uiex: can't setup HTTP client to %s: %w", baseUrl.String(), err)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	httpClient.Timeout = timeout

	userAgent := "flyctl"
	if opts.UserAgent != "" {
		userAgent = opts.UserAgent
	}

	return &Client{
		baseUrl:    baseUrl,
		tokens:     opts.Tokens,
		httpClient: httpClient,
		userAgent:  userAgent,
	}, nil
}

func (c *Client) BaseURL() *url.URL {
	return c.baseUrl
}

func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}
