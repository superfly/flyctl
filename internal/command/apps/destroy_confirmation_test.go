//go:build !windows

package apps

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunDestroyDeclined(t *testing.T) {
	cmd := newDestroy()
	require.NoError(t, cmd.ParseFlags([]string{"production", "other-app"}))
	ios, out, _ := mock.ConfirmationIO("n")
	ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
	calls := 0
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
		DeleteAppFunc: func(context.Context, string) error { calls++; return nil },
	})
	require.NoError(t, RunDestroy(ctx))
	require.Zero(t, calls)
	require.NotContains(t, out.String(), "Destroyed app")
}
