package tracing

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

type readFunc func([]byte) (int, error)

func (f readFunc) Read(p []byte) (int, error) { return f(p) }

func TestTransportRecordsCancellationBeforeHTTPSpanEnds(t *testing.T) {
	for _, duringRead := range []bool{false, true} {
		name := "round trip"
		if duringRead {
			name = "body read"
		}
		t.Run(name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			defer provider.Shutdown(context.Background())
			ctx, cancel := context.WithCancelCause(context.Background())
			defer cancel(nil)
			cause := errors.New("sibling deployment failed")
			inner := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if !duringRead {
					cancel(cause)
					return nil, req.Context().Err()
				}
				body := io.NopCloser(readFunc(func([]byte) (int, error) { cancel(cause); return 0, req.Context().Err() }))

				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: body, Request: req}, nil
			})
			transport := otelhttp.NewTransport(NewTransport(inner), otelhttp.WithTracerProvider(provider))
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://example.test/", nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := transport.RoundTrip(req)
			if duringRead {
				if err != nil {
					t.Fatal(err)
				}
				_, err = io.ReadAll(resp.Body)
				resp.Body.Close()
			}
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("request error=%v", err)
			}
			spans := recorder.Ended()
			if len(spans) != 1 || attrValue(spans[0].Attributes(), "context.cause") != cause.Error() {
				t.Fatalf("HTTP cancellation cause missing: %v", spans)
			}
		})
	}
}
