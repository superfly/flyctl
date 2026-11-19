package tracing

import (
	"context"
	"io"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

func NewTransport(inner http.RoundTripper) http.RoundTripper {
	if inner == nil {
		inner = http.DefaultTransport
	}
	return &InstrumentedTransport{
		inner: inner,
	}
}

type InstrumentedTransport struct {
	inner http.RoundTripper
}

func (t *InstrumentedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	gp := otel.GetTextMapPropagator()
	req = req.Clone(req.Context())

	gp.Inject(req.Context(), propagation.HeaderCarrier(req.Header))

	resp, err := t.inner.RoundTrip(req)
	if err != nil {
		RecordCancellation(req.Context(), trace.SpanFromContext(req.Context()))
		return resp, err
	}

	if resp != nil && resp.Body != nil {
		resp.Body = &cancellationBody{ReadCloser: resp.Body, ctx: req.Context()}
	}

	return resp, err
}

// Observe cancellation during response reads before otelhttp ends the HTTP span.
type cancellationBody struct {
	io.ReadCloser
	ctx context.Context
}

func (b *cancellationBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		RecordCancellation(b.ctx, trace.SpanFromContext(b.ctx))
	}
	return n, err
}
