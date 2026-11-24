package cli_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/cli"
)

func TestMain(m *testing.M) {
	// The trace exporter owns its HTTP client and bypasses DefaultTransport.
	// "127.0.0.1:1" is not a valid collector URL (it fails URL parsing), so the
	// exporter never dials anything. Apply this in every test process,
	// including subprocess helpers.
	if err := os.Setenv("FLY_TRACE_COLLECTOR_URL", "127.0.0.1:1"); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func TestVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
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
