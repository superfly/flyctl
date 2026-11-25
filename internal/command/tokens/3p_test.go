package tokens

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
	flytokens "github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
)

func TestRun3PAddReportsFailure(t *testing.T) {
	const location = "https://third-party.example"
	t.Setenv("TOKEN_3P_SHARED_SECRET", base64.StdEncoding.EncodeToString(macaroon.NewEncryptionKey()))

	// A token can hold only one caveat per third-party location.
	encoded, err := macaroon.Parse(permissionToken(t))
	require.NoError(t, err)
	m, err := macaroon.Decode(encoded[0])
	require.NoError(t, err)
	require.NoError(t, m.Add3P(macaroon.NewEncryptionKey(), location))
	tok, err := m.Encode()
	require.NoError(t, err)

	cmd := new3PAdd()
	require.NoError(t, cmd.ParseFlags([]string{"--location", location}))
	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
	ctx = config.NewContext(ctx, &config.Config{Tokens: flytokens.Parse(macaroon.ToAuthorizationHeader(tok))})

	require.ErrorContains(t, run3PAdd(ctx), "failed to add third-party caveat")
}

func TestReadonlyFromExistingWithoutPermissionToken(t *testing.T) {
	m, err := macaroon.New([]byte("test-token"), "https://not-permission.example", macaroon.NewSigningKey())
	require.NoError(t, err)
	tok, err := m.Encode()
	require.NoError(t, err)

	cmd := newOrgRead()
	require.NoError(t, cmd.ParseFlags([]string{"--from-existing"}))
	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(flag.NewContext(t.Context(), cmd.Flags()), ios)
	ctx = config.NewContext(ctx, &config.Config{Tokens: flytokens.Parse(macaroon.ToAuthorizationHeader(tok))})

	require.ErrorContains(t, runOrgRead(ctx), "no permission token")
}
