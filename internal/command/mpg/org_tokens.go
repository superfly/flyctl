package mpg

import (
	"context"
	"strings"

	clientsignals "github.com/superfly/client-signals/go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	uiexmpg "github.com/superfly/flyctl/internal/uiex/mpg"
	"github.com/superfly/macaroon/flyio"
)

// orgScopedTokens returns a copy of t whose macaroons are narrowed to the
// permission token(s) scoped to orgID plus their discharge tokens. User (OAuth)
// tokens are kept as-is.
//
// A user's session token carries one permission token, and its discharges, per
// organization they belong to. For users in many organizations the full bundle
// can exceed the request header limit of the Managed Postgres endpoints on the
// Machines API, which then answer 431 Request Header Fields Too Large.
//
// It returns nil if t holds no permission token for orgID (for example an org
// or app deploy token, or an empty bundle), in which case callers should keep
// using t unchanged.
func orgScopedTokens(t *tokens.Tokens, orgID uint64) *tokens.Tokens {
	if t == nil || orgID == 0 {
		return nil
	}

	macs := t.GetMacaroonTokens()
	if len(macs) == 0 {
		return nil
	}

	// The returned error describes malformed tokens; the bundle is still
	// usable and the default filter drops them.
	bun, _ := flyio.ParseBundle(strings.Join(macs, ","))

	scoped := bun.Select(bun.WithDischarges(flyio.IsForOrgUnverified(orgID)))
	if !scoped.Any(flyio.IsPermissionToken) {
		return nil
	}

	// Nothing to gain if every token is already for this org.
	if scoped.Len() == len(macs) {
		return nil
	}

	out := t.Copy()
	out.ReplaceMacaroonTokens(strings.Split(scoped.String(), ","))

	return out
}

// withOrgScopedTokens returns a context whose Managed Postgres requests (the
// Machines API client and the legacy MPG client) send only the tokens scoped to
// orgID. If no narrower token set exists, ctx is returned unchanged.
func withOrgScopedTokens(ctx context.Context, orgID uint64) (context.Context, error) {
	scoped := orgScopedTokens(config.Tokens(ctx), orgID)
	if scoped == nil {
		return ctx, nil
	}

	signals := clientsignals.DetectOnce()
	client, err := flapsutil.NewClientWithOptions(ctx, flaps.NewClientOpts{
		Tokens:        scoped,
		ClientSignals: &signals,
	})
	if err != nil {
		return ctx, err
	}

	ctx = flapsutil.NewContextWithClient(ctx, client)

	return uiexmpg.NewContextWithTokens(ctx, scoped), nil
}
