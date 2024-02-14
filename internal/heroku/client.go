package heroku

import (
	"errors"
	"net/http"
	"time"

	"github.com/PuerkitoBio/rehttp"
	heroku "github.com/heroku/heroku-go/v5"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
	"github.com/superfly/tokenizer"
)

type Client struct {
	*heroku.Service
}

func New(auth Auth) (*Client, error) {
	ht := &heroku.Transport{}
	if err := auth(ht); err != nil {
		return nil, err
	}

	retry := rehttp.NewTransport(ht,
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
	}, nil
}

type Auth func(ht *heroku.Transport) error

func BearerTokenAuth(token string) Auth {
	return func(ht *heroku.Transport) error {
		ht.BearerToken = token
		return nil
	}
}

func TokenizerAuth(sealedToken, auth string) Auth {
	return func(ht *heroku.Transport) error {
		rt := ht.Transport
		if rt == nil {
			rt = http.DefaultTransport
		}

		t, ok := rt.(*http.Transport)
		if !ok {
			return errors.New("can't use non *http.Transport")
		}
		t = t.Clone()

		tkzt, err := tokenizer.Transport(
			"https://tokenizer.fly.io",
			tokenizer.WithTransport(t),
			tokenizer.WithSecret(sealedToken, nil),
			tokenizer.WithAuth(auth),
		)
		if err != nil {
			return err
		}

		ht.Transport = tkzt
		return nil
	}
}
