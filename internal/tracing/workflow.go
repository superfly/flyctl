package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type workflowContextKey struct{}

// RunWorkflow traces a single command with the existing flyctl trace exporter.
// An unavailable exporter does not prevent the command from running.
func RunWorkflow(ctx context.Context, name, app string, run func(context.Context) error) error {
	provider, err := InitTraceProvider(ctx, app)
	if err != nil {
		return run(ctx)
	}
	ctx, span := CMDSpan(ctx, name)
	err = run(context.WithValue(ctx, workflowContextKey{}, true))
	finishWorkflowSpan(span, err)
	flush, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = provider.Shutdown(flush)

	return err
}

// WorkflowStep records a phase of a traced workflow. Its error is conveyed as a
// status because command errors can contain secrets or connection strings.
func WorkflowStep[T any](ctx context.Context, name string, run func(context.Context) (T, error)) (T, error) {
	if ctx.Value(workflowContextKey{}) == nil {
		return run(ctx)
	}
	ctx, span := CMDSpan(ctx, "workflow."+name)
	result, err := run(ctx)
	finishWorkflowSpan(span, err)

	return result, err
}

func WorkflowPhase(ctx context.Context, name string, run func(context.Context) error) error {
	_, err := WorkflowStep(ctx, name, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, run(ctx)
	})
	return err
}

func finishWorkflowSpan(span trace.Span, err error) {
	defer span.End()
	if err != nil {
		span.SetStatus(codes.Error, "workflow operation failed")
	} else {
		span.SetStatus(codes.Ok, "")
	}
}
