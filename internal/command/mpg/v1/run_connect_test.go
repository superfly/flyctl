package cmdv1

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
	"github.com/superfly/flyctl/iostreams"
)

func TestMaybeWarnNotReady(t *testing.T) {
	const name = "test-cluster"
	tests := []struct {
		name     string
		status   string
		wantWarn bool
	}{
		{name: "ready silent", status: "ready", wantWarn: false},
		{name: "creating warns", status: "creating", wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cluster := &mpgv1.ManagedCluster{Name: name, Status: tt.status}
			maybeWarnNotReady(&buf, cluster)

			got := buf.String()
			if !tt.wantWarn {
				require.Empty(t, got, "no warning expected for status=%q", tt.status)

				return
			}
			// Check the warning text independently of ANSI color codes.
			require.Contains(t, got, "WARN", "warning must contain the literal 'WARN' marker")
			require.Contains(t, got, "Cluster is not in ready state, currently: "+tt.status)
			require.True(t, bytes.HasSuffix(buf.Bytes(), []byte("\n")), "warning must end with a newline (pre-migration format)")
		})
	}
}

func TestRunConnectScriptedSkipsPickers(t *testing.T) {
	for _, name := range []string{"command", "file"} {
		t.Run(name, func(t *testing.T) {
			streams, _, _, _ := iostreams.Test()
			streams.SetStdinTTY(true)
			streams.SetStdoutTTY(true)
			ctx := iostreams.NewContext(context.Background(), streams)
			fs := pflag.NewFlagSet("connect", pflag.ContinueOnError)
			fs.String("username", "", "")
			fs.String("database", "", "")
			fs.StringArray("command", nil, "")
			fs.StringArray("file", nil, "")
			require.NoError(t, fs.Set(name, "sql"))
			ctx = flagctx.NewContext(ctx, fs)
			ctx = mpgv1.NewContextWithClient(ctx, &mock.MpgV1Client{
				ListUsersFunc: func(context.Context, string) (mpgv1.ListUsersResponse, error) {
					t.Fatal("user picker called")
					return mpgv1.ListUsersResponse{}, nil
				},
				ListDatabasesFunc: func(context.Context, string) (mpgv1.ListDatabasesResponse, error) {
					t.Fatal("database picker called")
					return mpgv1.ListDatabasesResponse{}, nil
				},
			})
			called := false
			ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
				GetManagedPostgresClusterFunc: func(context.Context, string) (flaps.ManagedPostgresCluster, error) {
					called = true
					return flaps.ManagedPostgresCluster{}, errors.New("stop before network")
				},
			})
			err := RunConnect(ctx, "cluster", "org", "psql")
			require.ErrorContains(t, err, "stop before network")
			require.True(t, called)
		})
	}
}
