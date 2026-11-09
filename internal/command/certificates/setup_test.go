package certificates

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func setupResponse() *fly.CertificateDetailResponse {
	return &fly.CertificateDetailResponse{
		Hostname:      "www.example.com",
		Status:        "pending",
		AcmeRequested: true,
		DNSRequirements: fly.DNSRequirements{
			A:     []string{"66.241.124.1"},
			AAAA:  []string{"2a09:8280:1::1"},
			CNAME: "my-app.fly.dev",
		},
	}
}

// setupContext wires everything runCertificatesSetup reads: the flaps client,
// the app name, the hostname argument and the --json setting.
func setupContext(t *testing.T, jsonOutput bool, check func(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error)) (context.Context, func() string, func() string) {
	t.Helper()

	cmd := newCertificatesSetup()
	require.NoError(t, cmd.Flags().Parse([]string{"www.example.com"}))

	ios, _, stdout, stderr := iostreams.Test()

	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = appconfig.WithName(ctx, "my-app")
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{CheckCertificateFunc: check})

	return ctx, stdout.String, stderr.String
}

func TestCertificatesSetupJSON(t *testing.T) {
	want := setupResponse()

	var gotApp, gotHost string
	ctx, stdout, _ := setupContext(t, true, func(_ context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error) {
		gotApp, gotHost = appName, hostname
		return want, nil
	})

	require.NoError(t, runCertificatesSetup(ctx))

	assert.Equal(t, "my-app", gotApp)
	assert.Equal(t, "www.example.com", gotHost)

	// stdout must be exactly one JSON document, with none of the human
	// instructions mixed in, so scripts can pipe it straight into a parser.
	var got fly.CertificateDetailResponse
	require.NoError(t, json.Unmarshal([]byte(stdout()), &got), "stdout is not valid JSON:\n%s", stdout())
	assert.Equal(t, *want, got)
	assert.NotContains(t, stdout(), "DNS Setup Options")
}

func TestCertificatesSetupHumanOutput(t *testing.T) {
	ctx, stdout, _ := setupContext(t, false, func(context.Context, string, string) (*fly.CertificateDetailResponse, error) {
		return setupResponse(), nil
	})

	require.NoError(t, runCertificatesSetup(ctx))

	out := stdout()
	assert.Contains(t, out, "my-app.fly.dev", "human output should include the CNAME target")
	assert.Contains(t, out, "66.241.124.1", "human output should include the A record")
	assert.False(t, json.Valid([]byte(out)), "human output should not be JSON")
}

func TestCertificatesSetupErrorWritesNothing(t *testing.T) {
	for _, jsonOutput := range []bool{true, false} {
		ctx, stdout, _ := setupContext(t, jsonOutput, func(context.Context, string, string) (*fly.CertificateDetailResponse, error) {
			return nil, errors.New("certificate not found")
		})

		err := runCertificatesSetup(ctx)
		require.EqualError(t, err, "certificate not found")
		assert.Empty(t, stdout(), "jsonOutput=%v", jsonOutput)
	}
}
