package metrics

import (
	"context"
	"errors"
	"fmt"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/terminal"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/bundle"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/resset"
)

func queryMetricsToken(ctx context.Context) (string, error) {
	// Manually construct an API client with the user's access token.
	// We use this over the context API client because we're trying to
	// authenticate the human user, not the specific credentials they're using.
	cfg := config.FromContext(ctx)
	apiClient := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
		Tokens: cfg.Tokens,
	})

	personal, err := apiClient.GetOrganizationBySlug(ctx, "personal")
	if err != nil {
		return "", err
	}
	if personal == nil {
		return "", errors.New("no personal organization found")
	}

	resp, err := gql.CreateLimitedAccessToken(
		ctx,
		apiClient.GenqClient(),
		"flyctl-metrics",
		personal.ID,
		"identity",
		struct{}{},
		"",
	)
	if err != nil {
		return "", fmt.Errorf("failed creating identity token: %w", err)
	}
	return resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader, nil
}

func GetMetricsToken(parentCtx context.Context) (token string, err error) {
	// Prevent metrics panics from bubbling up to the user.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()

	cfg := config.FromContext(parentCtx)
	if cfg.MetricsToken != "" {
		terminal.Debugf("Config has metrics token\n")
		return cfg.MetricsToken, nil
	}

	// if we have macaroons already, grab the first one and strip away all of
	// its permissions and use that as a metrics token.
	if macs := cfg.Tokens.MacaroonsOnly(); !macs.Empty() {
		cav := resset.ActionNone
		hdr := attenuatedFirstPermissionTokenWithDischarges(macs.All(), &cav)

		if hdr != "" {
			// don't persist this token, since it will likely expire soon and
			// it's easy to recreate
			return hdr, nil
		}
	}

	if cfg.MetricsToken == "" && cfg.Tokens.GraphQL() != "" {
		terminal.Debugf("Querying metrics token from web\n")
		token, err := queryMetricsToken(parentCtx)
		if err != nil {
			return "", err
		}
		if err = persistMetricsToken(parentCtx, token); err != nil {
			return "", err
		}
		cfg.MetricsToken = token
		return token, nil
	}
	return "", errors.New("no metrics token in config")
}

// attenuatedFirstPermissionTokenWithDischarges selects the first permission
// token and associated discharge tokens, applies the given caveats to the
// permission token, and returns the result as a token header.
func attenuatedFirstPermissionTokenWithDischarges(hdr string, caveats ...macaroon.Caveat) string {
	// selects the first permission token
	predicate := bundle.And(flyio.IsPermissionToken, firstToken())

	// selects permission tokens matching the predicate along with
	// associated discharge tokens
	filter := bundle.DefaultFilter(predicate)

	bun, _ := flyio.ParseBundleWithFilter(hdr, filter)

	if err := bun.Attenuate(caveats...); err != nil {
		return ""
	}

	return bun.Header()
}

// firstToken returns a predicate (macaroon filter function) that keeps only the
// first token passed to it.
func firstToken() bundle.Predicate {
	var first bundle.Token
	return func(t bundle.Token) bool {
		switch {
		case first == nil:
			first = t
			return true
		case first == t:
			return true
		default:
			return false
		}
	}
}

func persistMetricsToken(ctx context.Context, token string) error {
	path := state.ConfigFile(ctx)

	if err := config.SetMetricsToken(path, token); err != nil {
		return fmt.Errorf("failed persisting %s in %s: %w\n",
			config.MetricsTokenFileKey, path, err)
	}
	return nil
}
