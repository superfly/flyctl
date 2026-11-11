package mpg

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

// mintOrgToken returns a permission token scoped to orgID with a third-party
// caveat for the auth service, and the discharge token satisfying it.
func mintOrgToken(t *testing.T, orgID uint64) (perm, discharge string) {
	t.Helper()

	authKey := macaroon.NewEncryptionKey()

	m, err := macaroon.New([]byte("kid"), flyio.LocationPermission, macaroon.NewSigningKey())
	require.NoError(t, err)
	require.NoError(t, m.Add(&flyio.Organization{ID: orgID, Mask: resset.ActionAll}))
	require.NoError(t, m.Add3P(authKey, flyio.LocationAuthentication))

	perm, err = m.String()
	require.NoError(t, err)

	ticket, err := m.ThirdPartyTicket(flyio.LocationAuthentication)
	require.NoError(t, err)
	_, dm, err := macaroon.DischargeTicket(authKey, flyio.LocationAuthentication, ticket)
	require.NoError(t, err)

	discharge, err = dm.String()
	require.NoError(t, err)

	return perm, discharge
}

func TestOrgScopedTokens(t *testing.T) {
	perm1, dis1 := mintOrgToken(t, 1)
	perm2, dis2 := mintOrgToken(t, 2)
	perm3, dis3 := mintOrgToken(t, 3)

	all := tokens.Parse(strings.Join([]string{perm1, dis1, perm2, dis2, perm3, dis3, "fo1_oauth"}, ","))

	t.Run("narrows to the org's permission token and its discharge", func(t *testing.T) {
		scoped := orgScopedTokens(all, 2)
		require.NotNil(t, scoped)
		require.Equal(t, []string{perm2, dis2}, scoped.GetMacaroonTokens())
		require.Equal(t, []string{"fo1_oauth"}, scoped.GetUserTokens())
		require.Equal(t, "FlyV1 "+perm2+","+dis2, scoped.FlapsHeader())

		// The original is untouched.
		require.Len(t, all.GetMacaroonTokens(), 6)
	})

	t.Run("no token for the org leaves tokens unchanged", func(t *testing.T) {
		require.Nil(t, orgScopedTokens(all, 99))
	})

	t.Run("already scoped to a single org", func(t *testing.T) {
		require.Nil(t, orgScopedTokens(tokens.Parse(perm1+","+dis1), 1))
	})

	t.Run("no macaroons", func(t *testing.T) {
		require.Nil(t, orgScopedTokens(tokens.Parse("fo1_oauth"), 1))
	})

	t.Run("nil tokens or unknown org id", func(t *testing.T) {
		require.Nil(t, orgScopedTokens(nil, 1))
		require.Nil(t, orgScopedTokens(all, 0))
	})
}
