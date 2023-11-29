package tokens

import (
	"strings"
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
	FromConfigFile bool
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
