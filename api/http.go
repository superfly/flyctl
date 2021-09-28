package api

import (
	"bytes"
	"context"
	"io/ioutil"
	"math"
	"net/http"
	"time"

	"github.com/PuerkitoBio/rehttp"
)

var retryErrors = []string{"INTERNAL_ERROR", "read: connection reset by peer"}

func newHTTPClient() (*http.Client, error) {
	retryTransport := rehttp.NewTransport(
		http.DefaultTransport,
		rehttp.RetryAll(
			rehttp.RetryMaxRetries(3),
			rehttp.RetryAny(
				rehttp.RetryTemporaryErr(),
				rehttp.RetryStatuses(502, 503),
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
	logger         Logger
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
	t.logger.Debugf("--> %s %s %s\n", req.Method, req.URL, req.Body)
}

func (t *LoggingTransport) logResponse(resp *http.Response) {
	ctx := resp.Request.Context()
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.logger.Debug("error reading response body:", err)
	}
	if start, ok := ctx.Value(contextKeyRequestStart).(time.Time); ok {
		t.logger.Debugf("<-- %d %s (%s) %s\n", resp.StatusCode, resp.Request.URL, Duration(time.Since(start), 2), string(data))
	} else {
		t.logger.Debugf("<-- %d %s %s %s\n", resp.StatusCode, resp.Request.URL, string(data))
	}

	resp.Body = ioutil.NopCloser(bytes.NewReader(data))
}

func Duration(d time.Duration, dicimal int) time.Duration {
	shift := int(math.Pow10(dicimal))

	units := []time.Duration{time.Second, time.Millisecond, time.Microsecond, time.Nanosecond}
	for _, u := range units {
		if d > u {
			div := u / time.Duration(shift)
			if div == 0 {
				break
			}
			d = d / div * div
			break
		}
	}

	return d
}
