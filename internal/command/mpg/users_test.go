package mpg

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunUsersRotatePasswordJSONRequiresClusterBeforePicker(t *testing.T) {
	io, _, stdout, stderr := iostreams.Test()
	io.SetStdinTTY(true)
	io.SetStdoutTTY(true)
	ctx := iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: true})
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	ctx = flagctx.NewContext(ctx, flags)
	// No client context: reaching lookup or selection before validating the
	// arguments would panic, as well as potentially corrupting JSON stdout.
	err := runUsersRotatePassword(ctx)
	require.ErrorContains(t, err, "CLUSTER_ID must be specified when using --json")
	require.Empty(t, stdout.String())
	require.Empty(t, stderr.String())
}
