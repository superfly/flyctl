package cli_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/cli"
	"github.com/superfly/flyctl/internal/state"
)

func TestVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	dir, err := helpers.GetStateDirectory()
	assert.NoError(t, err)
	ctx = state.WithStateDirectory(ctx, dir)
	dir, err = helpers.GetRuntimeDirectory()
	assert.NoError(t, err)
	ctx = state.WithRuntimeDirectory(ctx, dir)

	defer cancel()

	stdout, stderr, code := capture(ctx, t, "version")
	assert.Equal(t, 0, code)

	assert.Equal(t, 0, code)
	assert.NotEmpty(t, stdout)
	assert.Empty(t, stderr)
}

func capture(ctx context.Context, t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()

	var o1, o2 strings.Builder

	io := iostreams.IOStreams{
		In:     nil,
		Out:    &o1,
		ErrOut: &o2,
	}

	code = cli.Run(ctx, &io, args...)
	stdout = o1.String()
	stderr = o2.String()

	return
}
