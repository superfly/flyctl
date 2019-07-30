package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
)

func newHTTPClient() (*http.Client, error) {
	var transport http.RoundTripper

	if viper.GetBool(flyctl.ConfigTrace) {
		transport = &LoggingTransport{
			innerTransport: http.DefaultTransport,
		}
	} else {
		transport = http.DefaultTransport
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
	log.Printf("--> %s %s %s", req.Method, req.URL, req.Body)
}

func (t *LoggingTransport) logResponse(resp *http.Response) {
	ctx := resp.Request.Context()
	if start, ok := ctx.Value(contextKeyRequestStart).(time.Time); ok {
		log.Printf("<-- %d %s (%s)", resp.StatusCode, resp.Request.URL, helpers.Duration(time.Now().Sub(start), 2))
	} else {
		log.Printf("<-- %d %s", resp.StatusCode, resp.Request.URL)
	}
}
