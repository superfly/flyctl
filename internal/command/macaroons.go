package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/skratchdot/open-golang/open"
	"github.com/superfly/flyctl/api/tokens"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/tp"
	"golang.org/x/exp/slices"
)

// dischargeThirdPartyCaveats attempts to fetch any necessary discharge tokens
// for 3rd party caveats found within macaroon tokens.
//
// See https://github.com/superfly/macaroon/blob/main/tp/README.md
func dischargeThirdPartyCaveats(ctx context.Context, t *tokens.Tokens) (bool, error) {
	macaroons := t.Macaroons()
	if macaroons == "" {
		return false, nil
	}

	opts := []tp.ClientOption{tp.WithUserURLCallback(tryOpenUserURL)}
	if len(t.UserTokens) != 0 {
		opts = append(opts, tp.WithBearerAuthentication(
			"auth.fly.io",
			strings.Join(t.UserTokens, ","),
		))
	}
	c := flyio.DischargeClient(opts...)

	switch needDischarge, err := c.NeedsDischarge(macaroons); {
	case err != nil:
		return false, err
	case !needDischarge:
		return false, nil
	}

	logger.FromContext(ctx).Debug("Attempting to upgrade authentication token...")

	withDischarges, err := c.FetchDischargeTokens(ctx, macaroons)

	// withDischarges will be non-empty in the event of partial success
	if withDischarges != "" && withDischarges != macaroons {
		t.MacaroonTokens = tokens.Parse(withDischarges).MacaroonTokens
		return true, err
	}

	return false, err
}

// pruneBadMacaroons removes expired and invalid macaroon tokens.
func pruneBadMacaroons(t *tokens.Tokens) bool {
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

		if expired := time.Now().After(m.Expiration()); expired {
			updated = true
			return true
		}

		return false
	})

	return updated
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
