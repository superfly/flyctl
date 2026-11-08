package tracing

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/baggage"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const deployTraceKey = "flyctl.deploy"

// WithDeployTrace marks this deploy and downstream requests for unconditional retention.
func WithDeployTrace(ctx context.Context) context.Context {
	member, _ := baggage.NewMember(deployTraceKey, "true")
	b, _ := baggage.FromContext(ctx).SetMember(member)
	return baggage.ContextWithBaggage(ctx, b)
}

// Copy the marker onto every span, including spans ending long after the initial request.
type deploySpanProcessor struct{}

func (deploySpanProcessor) OnStart(ctx context.Context, span sdktrace.ReadWriteSpan) {
	if baggage.FromContext(ctx).Member(deployTraceKey).Value() == "true" {
		span.SetAttributes(attribute.String(deployTraceKey, "true"))
	}
}
func (deploySpanProcessor) OnEnd(sdktrace.ReadOnlySpan)      {}
func (deploySpanProcessor) Shutdown(context.Context) error   { return nil }
func (deploySpanProcessor) ForceFlush(context.Context) error { return nil }
