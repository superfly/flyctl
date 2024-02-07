package tokens

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/tp"
	"golang.org/x/exp/slices"
)

// Tokens is a collection of tokens belonging to the user. This includes
// macaroon tokens (per-org) and OAuth tokens (per-user).
//
// It is normal for this to include just macaroons, just oauth tokens, or a
// combination of the two. The GraphQL API is the only service that accepts
// macaroons and OAuth tokens in the same request. For other service, macaroons
// are preferred.
type Tokens struct {
	MacaroonTokens []string
	UserTokens     []string
	FromConfigFile string
}

// Parse extracts individual tokens from a token string. The input token may
// include an authorization scheme (`Bearer` or `FlyV1`) and/or a set of
// comma-separated macaroon and user tokens.
func Parse(token string) *Tokens {
	token = stripAuthorizationScheme(token)
	ret := &Tokens{}

	for _, tok := range strings.Split(token, ",") {
		tok = strings.TrimSpace(tok)
		switch pfx, _, _ := strings.Cut(tok, "_"); pfx {
		case "fm1r", "fm1a", "fm2":
			ret.MacaroonTokens = append(ret.MacaroonTokens, tok)
		default:
			ret.UserTokens = append(ret.UserTokens, tok)
		}
	}

	return ret
}

// Update prunes any invalid/expired macaroons and fetches needed third party
// discharges
func (t *Tokens) Update(ctx context.Context, opts ...UpdateOption) (bool, error) {
	pruned := t.pruneBadMacaroons()
	discharged, err := t.dischargeThirdPartyCaveats(ctx, opts)

	return pruned || discharged, err
}

func (t *Tokens) Flaps() string {
	return t.normalized(false, false)
}

func (t *Tokens) FlapsHeader() string {
	return t.normalized(false, true)
}

func (t *Tokens) Docker() string {
	return t.normalized(false, false)
}

func (t *Tokens) NATS() string {
	return t.normalized(false, false)
}

func (t *Tokens) Bubblegum() string {
	return t.normalized(false, false)
}

func (t *Tokens) BubblegumHeader() string {
	return t.normalized(false, true)
}

func (t *Tokens) GraphQL() string {
	return t.normalized(true, false)
}

func (t *Tokens) GraphQLHeader() string {
	return t.normalized(true, true)
}

func (t *Tokens) All() string {
	return t.normalized(true, false)
}

func (t *Tokens) Macaroons() string {
	return strings.Join(t.MacaroonTokens, ",")
}

func (t *Tokens) normalized(macaroonsAndUserTokens, includeScheme bool) string {
	scheme := ""
	if includeScheme {
		scheme = "Bearer "
		if len(t.MacaroonTokens) > 0 {
			scheme = "FlyV1 "
		}
	}

	if macaroonsAndUserTokens {
		return scheme + strings.Join(append(t.MacaroonTokens, t.UserTokens...), ",")
	}
	if len(t.MacaroonTokens) == 0 {
		return scheme + strings.Join(t.UserTokens, ",")
	}
	return scheme + t.Macaroons()
}

// stripAuthorizationScheme strips any FlyV1/Bearer schemes from token.
func stripAuthorizationScheme(token string) string {
	token = strings.TrimSpace(token)

	pfx, rest, found := strings.Cut(token, " ")
	if !found {
		return token
	}

	if pfx = strings.TrimSpace(pfx); strings.EqualFold(pfx, "Bearer") || strings.EqualFold(pfx, "FlyV1") {
		return stripAuthorizationScheme(rest)
	}

	return token
}

// pruneBadMacaroons removes expired and invalid macaroon tokens.
func (t *Tokens) pruneBadMacaroons() bool {
	var updated bool

	// TODO: remove unused discharge tokens

	t.MacaroonTokens = slices.DeleteFunc(t.MacaroonTokens, func(tok string) bool {
		raws, err := macaroon.Parse(tok)
		if err != nil {
			updated = true
			return true
		}

		m, err := macaroon.Decode(raws[0])
		if err != nil {
			updated = true
			return true
		}

		if time.Now().After(m.Expiration()) {
			updated = true
			return true
		}

		// preemptively prune auth tokens that will expire in the next minute.
		// The hope is that we can replace discharge tokens *before* they expire
		// so requests don't fail.
		//
		// TODO: this is hacky
		if m.Location == flyio.LocationAuthentication && time.Now().Add(time.Minute).After(m.Expiration()) {
			updated = true
			return true
		}

		return false
	})

	return updated
}

// dischargeThirdPartyCaveats attempts to fetch any necessary discharge tokens
// for 3rd party caveats found within macaroon tokens.
//
// See https://github.com/superfly/macaroon/blob/main/tp/README.md
func (t *Tokens) dischargeThirdPartyCaveats(ctx context.Context, opts []UpdateOption) (bool, error) {
	macaroons := t.Macaroons()
	if macaroons == "" {
		return false, nil
	}

	options := &updateOptions{debugger: noopDebugger{}}
	for _, o := range opts {
		o(options)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return false, err
	}

	h := &http.Client{Jar: jar}
	if options.debugger != nil {
		h.Transport = debugTransport{
			d: options.debugger,
			t: http.DefaultTransport,
		}
	}

	copts := options.clientOptions
	copts = append(copts, tp.WithHTTP(h))
	if len(t.UserTokens) != 0 {
		copts = append(copts,
			tp.WithBearerAuthentication(
				"auth.fly.io",
				strings.Join(t.UserTokens, ","),
			), tp.WithBearerAuthentication(
				flyio.LocationAuthentication,
				strings.Join(t.UserTokens, ","),
			))
	}
	c := flyio.DischargeClient(copts...)

	switch needDischarge, err := c.NeedsDischarge(macaroons); {
	case err != nil:
		return false, err
	case !needDischarge:
		return false, nil
	}

	options.debugger.Debug("Attempting to upgrade authentication token")
	withDischarges, err := c.FetchDischargeTokens(ctx, macaroons)

	// withDischarges will be non-empty in the event of partial success
	if withDischarges != "" && withDischarges != macaroons {
		t.MacaroonTokens = Parse(withDischarges).MacaroonTokens
		return true, err
	}

	return false, err
}

type UpdateOption func(*updateOptions)

type updateOptions struct {
	clientOptions []tp.ClientOption
	debugger      Debugger
}

func WithUserURLCallback(cb func(ctx context.Context, url string) error) UpdateOption {
	return func(o *updateOptions) {
		o.clientOptions = append(o.clientOptions, tp.WithUserURLCallback(cb))
	}
}

func WithDebugger(d Debugger) UpdateOption {
	return func(o *updateOptions) {
		o.debugger = d
	}
}

type Debugger interface {
	Debug(...any)
}

type noopDebugger struct{}

func (noopDebugger) Debug(...any) {}

type debugTransport struct {
	d Debugger
	t http.RoundTripper
}

func (d debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	d.d.Debug("Request:", req.URL.String())
	return d.t.RoundTrip(req)
}
