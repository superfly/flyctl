package heroku

import (
	"net/http"
	"time"

	"github.com/PuerkitoBio/rehttp"
	heroku "github.com/heroku/heroku-go/v5"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
)

type Client struct {
	*heroku.Service
}

func New(token string) *Client {
	heroku.DefaultTransport.BearerToken = token

	retry := rehttp.NewTransport(
		heroku.DefaultTransport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(3),
			rehttp.RetryAny(
				rehttp.RetryTemporaryErr(),
				rehttp.RetryStatuses(502, 503),
			),
		),
		rehttp.ExpJitterDelay(100*time.Millisecond, 1*time.Second),
	)

	logging := &api.LoggingTransport{
		InnerTransport: retry,
		Logger:         terminal.DefaultLogger,
	}

	httpClient := &http.Client{
		Transport: logging,
	}

	return &Client{
		Service: heroku.NewService(httpClient),
	}
}
