package tracing

import (
	"context"
	"os"

	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	traceparentEnv = "FLYCTL_TRACEPARENT"
	tracestateEnv  = "FLYCTL_TRACESTATE"
)

// ProcessEnvironment returns environment overrides for a child process.
func ProcessEnvironment(ctx context.Context) []string {
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	return []string{
		traceparentEnv + "=" + carrier.Get("traceparent"),
		tracestateEnv + "=" + carrier.Get("tracestate"),
	}
}

// ExtractProcessContext reads the remote parent supplied by a supervising process.
func ExtractProcessContext(ctx context.Context) (context.Context, bool) {
	parent := os.Getenv(traceparentEnv)
	if parent == "" {
		return ctx, false
	}
	extracted := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier{
		"traceparent": parent,
		"tracestate":  os.Getenv(tracestateEnv),
	})
	if sc := trace.SpanContextFromContext(extracted); sc.IsValid() {
		return trace.ContextWithRemoteSpanContext(ctx, sc), true
	}
	return ctx, false
}
