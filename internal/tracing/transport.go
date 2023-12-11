package tracing

import (
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

func NewTransport(inner http.RoundTripper) http.RoundTripper {
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
		return resp, err
	}

	return resp, err
}
