// Package authtest checks the wire credentials selected by command contexts.
package authtest

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/requestauth"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Organization returns a check using two real roots; the selected request must
// retain id and omit the unrelated org, without inspecting private context keys.
func Organization(t *testing.T, id uint64) func(context.Context) {
	t.Helper()
	var roots []string
	for _, org := range []uint64{id, id + 1} {
		m, err := macaroon.New([]byte("test"), flyio.LocationPermission, macaroon.NewSigningKey())
		require.NoError(t, err)
		require.NoError(t, m.Add(&flyio.Organization{ID: org, Mask: resset.ActionAll}))
		root, err := m.String()
		require.NoError(t, err)
		roots = append(roots, root)
	}
	return func(ctx context.Context) {
		t.Helper()
		r, err := http.NewRequestWithContext(ctx, "POST", "http://local", nil)
		require.NoError(t, err)
		r.Header.Set("Authorization", "FlyV1 "+strings.Join(roots, ","))
		_, _ = requestauth.Transport(roundTrip(func(r *http.Request) (*http.Response, error) {
			require.Equal(t, "FlyV1 "+roots[0], r.Header.Get("Authorization"))
			return nil, errors.New("stop before network")
		})).RoundTrip(r)
	}
}
