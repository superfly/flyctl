// Package requestauth selects credentials for requests with a known organization.
package requestauth

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/superfly/macaroon/bundle"
	"github.com/superfly/macaroon/flyio"
)

type organizationKey struct{}

// WithOrganizations replaces the request's scope. No IDs, or an unknown ID (0),
// leaves credentials unchanged. Callers must include every organization needed.
func WithOrganizations(ctx context.Context, ids ...uint64) context.Context {
	return context.WithValue(ctx, organizationKey{}, slices.Clone(ids))
}

type transport struct{ next http.RoundTripper }

func Transport(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return transport{next}
}

func (t transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ids, _ := req.Context().Value(organizationKey{}).([]uint64)
	if len(ids) == 0 || slices.Contains(ids, 0) {
		return t.next.RoundTrip(req)
	}
	header := req.Header.Get("Authorization")
	scheme, _, _ := strings.Cut(header, " ")
	if !strings.EqualFold(scheme, "Bearer") && !strings.EqualFold(scheme, "FlyV1") {
		return t.next.RoundTrip(req)
	}
	b, err := flyio.ParseBundleWithFilter(header, bundle.KeepAll)
	if err != nil {
		return t.next.RoundTrip(req)
	}
	// Do not turn missing org credentials into an OAuth-only request or remove
	// another required org's credentials when only part of the scope is present.
	found := make(map[uint64]bool, len(ids))
	keep := bundle.Or(bundle.IsNonMacaroon, bundle.MacaroonPredicate(func(m bundle.Macaroon) bool {
		if !flyio.IsPermissionToken(m) {
			return false
		}
		id, err := flyio.OrganizationScope(m.UnsafeCaveats())
		if err != nil || id == 0 {
			return true
		}
		if slices.Contains(ids, id) {
			found[id] = true
			return true
		}

		return false
	}))
	selected := b.Select(b.WithDischarges(keep))
	for _, id := range ids {
		if !found[id] {
			return t.next.RoundTrip(req)
		}
	}
	scoped := req.Clone(req.Context())
	scoped.Header.Set("Authorization", scheme+" "+selected.String())

	return t.next.RoundTrip(scoped)
}
