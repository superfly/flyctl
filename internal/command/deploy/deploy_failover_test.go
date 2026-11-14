package deploy

import (
	"context"
	"errors"
	"testing"

	gatewaypb "github.com/moby/buildkit/frontend/gateway/pb"
	pkgerrors "github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCanFailOverToHTTPS(t *testing.T) {
	ctx := context.Background()

	// The builder couldn't be reached over WireGuard: HTTPS might work.
	assert.True(t, canFailOverToHTTPS(ctx, errors.New("failed building options: tunnel unavailable")))

	// The builder ran the build and a RUN step failed: HTTPS would fail the same way.
	runFailed := pkgerrors.Wrap(&gatewaypb.ExitError{ExitCode: 1}, `process "/bin/sh -c npm run build" did not complete successfully`)
	assert.False(t, canFailOverToHTTPS(ctx, pkgerrors.Wrap(runFailed, "error building")))

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	assert.False(t, canFailOverToHTTPS(canceled, errors.New("error building: context canceled")))
}

func TestHTTPSFailoverError(t *testing.T) {
	wireguardErr := errors.New("remote builder app unavailable")

	// An older builder refused the HTTPS retry, so the WireGuard error is the one to fix.
	refused := errors.New(`connection error: desc = "transport: Error while dialing: unable to upgrade to h2c, received 500"`)
	err := httpsFailoverError(wireguardErr, refused)
	require.ErrorIs(t, err, wireguardErr)
	assert.Equal(t, "remote builder app unavailable\nRetrying the build over HTTPS failed too: "+refused.Error(), err.Error())

	// The retry reached the builder and failed there: that's the error that matters now.
	buildFailed := errors.New("error building: failed to solve: exit code: 1")
	assert.Equal(t, buildFailed, httpsFailoverError(wireguardErr, buildFailed))

	assert.NoError(t, httpsFailoverError(wireguardErr, nil))
}
