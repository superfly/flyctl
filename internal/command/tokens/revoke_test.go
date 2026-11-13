package tokens

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunRevokeBatch(t *testing.T) {
	denied := errors.New("permission denied")
	for _, tc := range []struct {
		name             string
		ids              []string
		fail             map[string]error
		wantOut, wantErr string
	}{
		{name: "missing IDs", wantErr: "no token IDs"},
		{name: "all succeed", ids: []string{"one", "two"}, wantOut: "Revoked one\nRevoked two\n2 tokens revoked\n"},
		{name: "all fail", ids: []string{"one", "two"}, fail: map[string]error{"one": denied, "two": denied}, wantOut: "0 tokens revoked\n", wantErr: "failed to revoke 2 of 2 tokens"},
		{name: "partial failure still attempts later IDs", ids: []string{"one", "two", "three"}, fail: map[string]error{"two": denied}, wantOut: "Revoked one\nRevoked three\n2 tokens revoked\n", wantErr: "failed to revoke 1 of 3 tokens"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := pflag.NewFlagSet("revoke", pflag.ContinueOnError)
			require.NoError(t, fs.Parse(tc.ids))
			ios, _, out, errOut := iostreams.Test()
			ctx := iostreams.NewContext(flag.NewContext(t.Context(), fs), ios)
			var called []string
			ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
				RevokeLimitedAccessTokenFunc: func(_ context.Context, id string) error {
					called = append(called, id)
					return tc.fail[id]
				},
			})
			err := runRevoke(ctx)
			if tc.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantErr)
			}
			require.Equal(t, tc.ids, called)
			require.Equal(t, tc.wantOut, out.String())
			for id := range tc.fail {
				require.ErrorIs(t, err, denied)
				require.Contains(t, errOut.String(), "failed to revoke token "+id+": permission denied")
				require.NotContains(t, out.String(), "Revoked "+id)
			}
		})
	}
}
