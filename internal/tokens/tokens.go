package tokens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/tp"
	"golang.org/x/exp/slices"
)

type Tokens struct {
	macaroonTokens []string
	userTokens     []string
}

func Parse(token string) *Tokens {
	token = stripAuthorizationScheme(token)
	ret := &Tokens{}

	for _, tok := range strings.Split(token, ",") {
		tok = strings.TrimSpace(tok)
		switch pfx, _, _ := strings.Cut(tok, "_"); pfx {
		case "fm1r", "fm1a", "fm2":
			ret.macaroonTokens = append(ret.macaroonTokens, tok)
		default:
			ret.userTokens = append(ret.userTokens, tok)
		}
	}

	return ret
}

func (t *Tokens) Flaps() string {
	return t.normalized(false)
}

func (t *Tokens) Docker() string {
	return t.normalized(false)
}

func (t *Tokens) NATS() string {
	return t.normalized(false)
}

func (t *Tokens) API() string {
	return t.normalized(true)
}

func (t *Tokens) All() string {
	return t.normalized(true)
}

func (t *Tokens) Macaroons() string {
	return strings.Join(t.macaroonTokens, ",")
}

var tpClient = &tp.Client{
	FirstPartyLocation: flyio.LocationPermission,
	UserURLCallback:    tryOpenUserURL,
}

func (t *Tokens) DischargeThirdPartyCaveats(ctx context.Context) (bool, error) {
	macaroons := t.Macaroons()
	if macaroons == "" {
		return false, nil
	}

	switch needDischarge, err := tpClient.NeedsDischarge(macaroons); {
	case err != nil:
		return false, err
	case !needDischarge:
		return false, nil
	}

	logger.FromContext(ctx).Debug("Attempting to upgrade authentication token...")

	withDischarges, err := tpClient.FetchDischargeTokens(ctx, macaroons)

	// withDischarges will be non-empty in the event of partial success
	if withDischarges != "" && withDischarges != macaroons {
		t.macaroonTokens = Parse(withDischarges).macaroonTokens
		return true, err
	}

	return false, err
}

// removes expired and invalid macaroons
func (t *Tokens) PruneBadMacaroons() bool {
	var updated bool

	t.macaroonTokens = slices.DeleteFunc(t.macaroonTokens, func(tok string) bool {
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

		if expired := time.Now().After(m.Expiration()); expired {
			updated = true
			return true
		}

		return false
	})

	return updated
}

func (t *Tokens) normalized(macaroonsAndUserTokens bool) string {
	if macaroonsAndUserTokens {
		return strings.Join(append(t.macaroonTokens, t.userTokens...), ",")
	}
	if len(t.macaroonTokens) == 0 {
		return strings.Join(t.userTokens, ",")
	}
	return t.Macaroons()
}

// strip any FlyV1/Bearer schemes from token.
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

func tryOpenUserURL(ctx context.Context, url string) error {
	if err := open.Run(url); err != nil {
		fmt.Fprintf(iostreams.FromContext(ctx).ErrOut,
			"failed opening browser. Copy the url (%s) into a browser and continue\n",
			url,
		)
	}

	return nil
}
