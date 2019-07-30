package api

import (
	"context"
	"net/http"
	"time"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/terminal"
)

func newHTTPClient() (*http.Client, error) {
	transport := &LoggingTransport{
		innerTransport: http.DefaultTransport,
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
