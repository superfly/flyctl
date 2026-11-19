// Package trace runs a subprocess within a flyctl trace.
package trace

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"go.opentelemetry.io/otel/attribute"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ExitError carries a child exit status through Cobra without printing the child's error again.
type ExitError struct{ Code int }

func (e *ExitError) Error() string { return fmt.Sprintf("child exited with status %d", e.Code) }

func New() *cobra.Command {
	group := &cobra.Command{Use: "trace", Hidden: true}
	run := command.New("run -- <command> [args...]", "Run a command within a trace", "", run)
	run.Args = cobra.MinimumNArgs(1)
	group.AddCommand(run)
	return group
}

func run(ctx context.Context) error {
	args := command.FromContext(ctx).Flags().Args()
	return tracing.RunWorkflow(tracing.WithDeployTrace(ctx), "deployer.run", os.Getenv("DEPLOY_APP_NAME"), func(ctx context.Context) error {
		return runChild(ctx, args)
	})
}

func runChild(ctx context.Context, args []string) error {
	io := iostreams.FromContext(ctx)
	child := exec.Command(args[0], args[1:]...)
	child.Stdin, child.Stdout, child.Stderr = io.In, io.Out, io.ErrOut
	child.Env = append(child.Environ(), tracing.ProcessEnvironment(ctx)...)
	forward := childSignalForwarder(child)

	signals := make(chan os.Signal, 2)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := child.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() { done <- child.Wait() }()

	canceled := false
	for {
		select {
		case sig := <-signals:
			canceled = true
			oteltrace.SpanFromContext(ctx).SetAttributes(attribute.String("deployer.cancel.signal", sig.String()))
			forward(sig)
		case err := <-done:
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return &ExitError{Code: childExitCode(exit)}
			}
			if err == nil && canceled {
				return &ExitError{Code: 0}
			}

			return err
		}
	}
}
