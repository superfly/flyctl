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
	"github.com/superfly/flyctl/internal/flag/flagnames"
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

// setupContext parses args with the real `certs setup` flag set and wires
// everything runCertificatesSetup reads: the flaps client, the app name, the
// hostname argument and the --json setting.
func setupContext(t *testing.T, args []string, check func(ctx context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error)) (ctx context.Context, stdout, stderr func() string) {
	t.Helper()

	// Parsing --json through the command's own flags fails if the flag is
	// ever dropped from newCertificatesSetup.
	cmd := newCertificatesSetup()
	require.NoError(t, cmd.Flags().Parse(args))
	jsonOutput, err := cmd.Flags().GetBool(flagnames.JSONOutput)
	require.NoError(t, err)

	ios, _, out, errOut := iostreams.Test()

	ctx = iostreams.NewContext(context.Background(), ios)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = appconfig.WithName(ctx, "my-app")
	ctx = flapsutil.NewContextWithClient(ctx, &mock.FlapsClient{CheckCertificateFunc: check})

	return ctx, out.String, errOut.String
}

func TestCertificatesSetupJSON(t *testing.T) {
	want := setupResponse()

	var gotApp, gotHost string
	ctx, stdout, stderr := setupContext(t, []string{"--json", "www.example.com"}, func(_ context.Context, appName, hostname string) (*fly.CertificateDetailResponse, error) {
		gotApp, gotHost = appName, hostname
		return want, nil
	})

	require.NoError(t, runCertificatesSetup(ctx))

	assert.Equal(t, "my-app", gotApp)
	assert.Equal(t, "www.example.com", gotHost)

	// stdout must be exactly one JSON document so scripts can pipe it straight
	// into a parser; Unmarshal rejects any human text before or after it.
	var got fly.CertificateDetailResponse
	require.NoError(t, json.Unmarshal([]byte(stdout()), &got), "stdout is not a single JSON document:\n%s", stdout())
	assert.Equal(t, *want, got)
	assert.Empty(t, stderr())
}

func TestCertificatesSetupHumanOutput(t *testing.T) {
	ctx, stdout, _ := setupContext(t, []string{"www.example.com"}, func(context.Context, string, string) (*fly.CertificateDetailResponse, error) {
		return setupResponse(), nil
	})

	require.NoError(t, runCertificatesSetup(ctx))

	out := stdout()
	assert.Contains(t, out, "DNS Setup Options")
	assert.Contains(t, out, "my-app.fly.dev", "human output should include the CNAME target")
	assert.Contains(t, out, "66.241.124.1", "human output should include the A record")
}

func TestCertificatesSetupErrorWritesNothing(t *testing.T) {
	for name, args := range map[string][]string{
		"json":  {"--json", "www.example.com"},
		"human": {"www.example.com"},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, stdout, _ := setupContext(t, args, func(context.Context, string, string) (*fly.CertificateDetailResponse, error) {
				return nil, errors.New("certificate not found")
			})

			assert.EqualError(t, runCertificatesSetup(ctx), "certificate not found")
			assert.Empty(t, stdout())
		})
	}
}
