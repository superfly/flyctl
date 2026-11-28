package diag

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

func TestRunReportsRejectedUpload(t *testing.T) {
	status := http.StatusForbidden
	uploads := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		uploads++
		w.WriteHeader(status)
	}))
	defer srv.Close()

	// The upload uses the default client, which doesn't trust the test server.
	transport := http.DefaultClient.Transport
	http.DefaultClient.Transport = srv.Client().Transport
	defer func() { http.DefaultClient.Transport = transport }()

	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("force", true, "")
	io, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(t.Context(), io)
	ctx = flag.NewContext(ctx, flags)
	ctx = state.WithConfigDirectory(ctx, t.TempDir())
	ctx = flyutil.NewContextWithClient(ctx, &mock.Client{
		CreateDoctorUrlFunc: func(context.Context) (string, error) {
			return srv.URL + "/123/abc-1.zip", nil
		},
	})

	require.ErrorContains(t, Run(ctx), "403")

	status = http.StatusOK
	require.NoError(t, Run(ctx))
	require.Equal(t, 2, uploads)
}
