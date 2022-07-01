package flaps

import (
	"bytes"
	"context"
	"io/ioutil"
	"math"
	"net/http"
	"time"
)

type Logger interface {
	Debug(v ...interface{})
	Debugf(format string, v ...interface{})
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
	if req.Body == nil {
		t.logger.Debugf("--> %s %s\n", req.Method, req.URL)
	} else {
		buf := new(bytes.Buffer)
		buf.ReadFrom(req.Body)
		req.Body = ioutil.NopCloser(buf)
		t.logger.Debugf("--> %s %s %s\n", req.Method, req.URL, buf.String())
	}
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
