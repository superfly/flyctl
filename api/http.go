package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/rehttp"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/terminal"
)

var retryErrors = []string{"INTERNAL_ERROR", "read: connection reset by peer"}

func newHTTPClient() (*http.Client, error) {
	retryTransport := rehttp.NewTransport(
		http.DefaultTransport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(3),
			rehttp.RetryAny(
				rehttp.RetryTemporaryErr(),
				rehttp.RetryIsErr(func(err error) bool {
					if err == nil {
						return true
					}
					msg := err.Error()
					for _, retryError := range retryErrors {
						if strings.Contains(msg, retryError) {
							return true
						}
					}
					return false
				}),
			),
		),
		rehttp.ExpJitterDelay(100*time.Millisecond, 1*time.Second),
	)

	transport := &LoggingTransport{
		innerTransport: retryTransport,
	}

	httpClient := &http.Client{
		Transport: transport,
	}

	return httpClient, nil
}

type LoggingTransport struct {
	innerTransport http.RoundTripper
}

type contextKey struct {
	name string
}

var contextKeyRequestStart = &contextKey{"RequestStart"}

func (t *LoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	ctx := context.WithValue(req.Context(), contextKeyRequestStart, time.Now())
	req = req.WithContext(ctx)

	t.logRequest(req)

	resp, err := t.innerTransport.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	t.logResponse(resp)

	return resp, err
}

func (t *LoggingTransport) logRequest(req *http.Request) {
	terminal.Debugf("--> %s %s %s\n", req.Method, req.URL, req.Body)
}

func (t *LoggingTransport) logResponse(resp *http.Response) {
	ctx := resp.Request.Context()
	if start, ok := ctx.Value(contextKeyRequestStart).(time.Time); ok {
		terminal.Debugf("<-- %d %s (%s)\n", resp.StatusCode, resp.Request.URL, helpers.Duration(time.Now().Sub(start), 2))
	} else {
		terminal.Debugf("<-- %d %s\n", resp.StatusCode, resp.Request.URL)
	}
}
