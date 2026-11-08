package tracing

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/baggage"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// Ensure the deploy marker survives HTTP propagation and reaches every span without marking unrelated commands.
func TestDeployTracePropagation(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(deploySpanProcessor{}), sdktrace.WithSpanProcessor(recorder))
	defer provider.Shutdown(context.Background())
	tracer := provider.Tracer("test")
	ctx, root := tracer.Start(WithDeployTrace(context.Background()), "cmd.deploy", trace.WithTimestamp(time.Now().Add(-time.Minute)))
	propagator := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	headers := propagation.MapCarrier{}
	propagator.Inject(ctx, headers)
	remote := propagator.Extract(context.Background(), headers)
	if baggage.FromContext(remote).Member(deployTraceKey).Value() != "true" {
		t.Fatal("deploy marker not propagated")
	}
	_, child := tracer.Start(remote, "backend", trace.WithTimestamp(time.Now().Add(-time.Minute)))
	child.End()
	root.End()
	_, unrelated := tracer.Start(context.Background(), "other", trace.WithTimestamp(time.Now().Add(-time.Minute)))
	unrelated.End()
	spans := recorder.Ended()
	if len(spans) != 3 {
		t.Fatalf("got %d spans", len(spans))
	}
	for i, span := range spans {
		marked := attrValue(span.Attributes(), deployTraceKey) == "true"
		if marked != (i < 2) {
			t.Fatalf("span %s marker=%v", span.Name(), marked)
		}
	}
	if spans[0].SpanContext().TraceID() != spans[1].SpanContext().TraceID() {
		t.Fatal("trace ID changed")
	}
}
