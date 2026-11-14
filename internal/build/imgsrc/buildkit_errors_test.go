package imgsrc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	gatewaypb "github.com/moby/buildkit/frontend/gateway/pb"
	"github.com/moby/buildkit/solver/errdefs"
	"github.com/moby/buildkit/util/grpcerrors"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fromBuilder sends err through gRPC the way a builder's BuildKit sends a
// failed solve to flyctl, then wraps it the way flyctl does on its way up to
// fly deploy.
func fromBuilder(err error) error {
	received := grpcerrors.FromGRPC(grpcerrors.ToGRPC(context.Background(), err))
	return fmt.Errorf("failed to fetch an image or build from source: %w", pkgerrors.Wrap(received, "error building"))
}

func TestIsBuildFailure(t *testing.T) {
	runFailed := pkgerrors.Wrapf(&gatewaypb.ExitError{ExitCode: 1}, "process %q did not complete successfully", "/bin/sh -c npm run build")

	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"RUN step exits non-zero", fromBuilder(runFailed), true},
		{"RUN step inside a solve error", fromBuilder(errdefs.WithSolveError(runFailed, nil, nil, nil)), true},
		// BuildKit wraps every solve failure as a SolveError, including losing
		// the session to flyctl, which another connection might fix.
		{"session lost during solve", fromBuilder(errdefs.WithSolveError(errors.New("error from sender: context canceled"), nil, nil, nil)), false},
		{"process never started", fromBuilder(&gatewaypb.ExitError{ExitCode: gatewaypb.UnknownExitStatus, Err: errors.New("failed to start")}), false},
		{"connection dropped", fromBuilder(status.Error(codes.Unavailable, "error reading from server: EOF")), false},
		{"builder unreachable", errors.New("failed building options: tunnel unavailable"), false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, IsBuildFailure(tc.err))
		})
	}

	assert.ErrorContains(t, fromBuilder(runFailed), `process "/bin/sh -c npm run build" did not complete successfully: exit code: 1`)
}

func TestIsBuilderHTTPSUnsupported(t *testing.T) {
	// gRPC reports the dialer's error inside its own connection error.
	refused := status.Error(codes.Unavailable, `connection error: desc = "transport: Error while dialing: unable to upgrade to h2c, received 500"`)

	assert.True(t, IsBuilderHTTPSUnsupported(fmt.Errorf("error building: %w", refused)))
	assert.False(t, IsBuilderHTTPSUnsupported(errors.New("error reading from server: EOF")))
	assert.False(t, IsBuilderHTTPSUnsupported(nil))
}
