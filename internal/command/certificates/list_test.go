package certificates

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"

	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestCertificatesListMorePages(t *testing.T) {
	const note = "Showing 1 of 60 certificates"

	for _, jsonOutput := range []bool{false, true} {
		io, _, out, errOut := iostreams.Test()
		ctx := iostreams.NewContext(t.Context(), io)
		ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
		ctx = appconfig.WithName(ctx, "test-app")
		ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{
			ListCertificatesFunc: func(context.Context, string, *flaps.ListCertificatesOpts) (*fly.ListCertificatesResponse, error) {
				return &fly.ListCertificatesResponse{
					Certificates: []fly.CertificateSummary{{Hostname: "example.com"}},
					NextCursor:   "next",
					TotalCount:   60,
				}, nil
			},
		})

		require.NoError(t, runCertificatesList(ctx))
		if jsonOutput {
			require.True(t, json.Valid(out.Bytes()), "stdout: %s", out)
			require.Contains(t, errOut.String(), note)
		} else {
			require.Contains(t, out.String(), note)
		}
	}
}
