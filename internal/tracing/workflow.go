package tracing

import (
	"context"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
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
	var once sync.Once
	finish := func(err error) {
		once.Do(func() {
			finishWorkflowSpan(span, err)
			flush, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			defer cancel()
			_ = provider.Shutdown(flush)
		})
	}
	err = run(context.WithValue(ctx, workflowContextKey{}, finish))
	finish(err)

	return err
}

// FinishWorkflow ends the observation when an interactive session starts.
func FinishWorkflow(ctx context.Context) {
	if finish, ok := ctx.Value(workflowContextKey{}).(func(error)); ok {
		finish(nil)
	}
}

// WorkflowStep records a phase of a traced workflow. Its error is conveyed as a
// status and a fixed error code because command errors can contain secrets or
// connection strings.
func WorkflowStep[T any](ctx context.Context, name string, run func(context.Context) (T, error)) (T, error) {
	ctx, finish := StartWorkflowPhase(ctx, name)
	result, err := run(ctx)
	finish(err)

	return result, err
}

// StartWorkflowPhase returns a completion function for a phase that finishes asynchronously.
func StartWorkflowPhase(ctx context.Context, name string) (context.Context, func(error)) {
	if ctx.Value(workflowContextKey{}) == nil {
		return ctx, func(error) {}
	}
	ctx, span := CMDSpan(ctx, "workflow."+name)
	var once sync.Once

	return ctx, func(err error) { once.Do(func() { finishWorkflowSpan(span, err) }) }
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
		span.SetAttributes(attribute.String("workflow.error.code", WorkflowErrorCodeFor(err).String()))
		span.SetStatus(codes.Error, "workflow operation failed")
	} else {
		span.SetStatus(codes.Ok, "")
	}
}
