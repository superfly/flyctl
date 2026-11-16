package tracing

import (
	"context"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestExtractProcessContextParentsSiblingSpans(t *testing.T) {
	t.Setenv(traceparentEnv, "00-12345678901234567890123456789012-1234567890123456-01")
	t.Setenv(tracestateEnv, "fly=value")
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	tracer := provider.Tracer("test")

	var siblingIDs []trace.SpanID
	for _, name := range []string{"build", "rollout"} {
		ctx, ok := ExtractProcessContext(context.Background())
		if !ok {
			t.Fatal("trace context was not extracted")
		}
		parent := trace.SpanContextFromContext(ctx)
		if !parent.IsRemote() || parent.SpanID().String() != "1234567890123456" || parent.TraceState().Get("fly") != "value" {
			t.Fatalf("wrong parent context: %v", parent)
		}
		_, span := tracer.Start(ctx, name)
		span.End()
		child := span.SpanContext()
		if child.TraceID() != parent.TraceID() || child.SpanID() == parent.SpanID() {
			t.Fatalf("%s did not inherit the parent trace: %v", name, child)
		}
		siblingIDs = append(siblingIDs, child.SpanID())
	}
	for _, span := range recorder.Ended() {
		if got := span.Parent().SpanID().String(); got != "1234567890123456" {
			t.Fatalf("%s parent span ID = %s", span.Name(), got)
		}
	}
	if siblingIDs[0] == siblingIDs[1] {
		t.Fatal("build and rollout got the same span ID")
	}

	t.Setenv(traceparentEnv, "malformed")
	ctx := context.Background()
	if got, ok := ExtractProcessContext(ctx); ok || got != ctx {
		t.Fatal("malformed context should be ignored")
	}
}
