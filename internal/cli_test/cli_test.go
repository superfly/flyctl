package cli_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli"
)

func TestVersion(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	stdout, stderr, code := capture(ctx, t, "version")
	assert.Equal(t, 0, code)

	assert.Equal(t, 0, code)
	assert.NotEmpty(t, stdout)
	assert.NotEmpty(t, stderr) // [33mWARN[0m no config file found at /home/azazeal/.fly/config.yml
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
