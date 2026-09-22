package uiex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestUpdateReleaseHTTPTracing(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previousProvider := otel.GetTracerProvider()
	previousPropagator := otel.GetTextMapPropagator()
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		require.NoError(t, provider.Shutdown(context.Background()))
	})

	received := make(chan trace.SpanContext, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/api/v1/releases/rel_test", r.URL.Path)
		ctx := propagation.TraceContext{}.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		received <- trace.SpanContextFromContext(ctx)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"rel_test","status":"running"}`))
	}))
	defer server.Close()
	baseURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	ctx := config.NewContext(context.Background(), &config.Config{Tokens: &tokens.Tokens{}})
	ctx, parent := provider.Tracer("test").Start(ctx, "update_release_in_backend")
	defer parent.End()
	client, err := NewWithOptions(ctx, NewClientOpts{BaseURL: baseURL})
	require.NoError(t, err)
	_, err = client.UpdateRelease(ctx, "rel_test", "running", nil)
	require.NoError(t, err)

	spans := recorder.Ended()
	require.Len(t, spans, 1)
	child := spans[0]
	require.Equal(t, trace.SpanKindClient, child.SpanKind())
	require.Equal(t, parent.SpanContext().SpanID(), child.Parent().SpanID())
	require.Equal(t, parent.SpanContext().TraceID(), child.SpanContext().TraceID())
	propagated := <-received
	require.True(t, propagated.IsValid())
	require.Equal(t, child.SpanContext().TraceID(), propagated.TraceID())
	require.Equal(t, child.SpanContext().SpanID(), propagated.SpanID())
}
