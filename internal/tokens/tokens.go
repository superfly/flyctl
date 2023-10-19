package tokens

import "strings"

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

func (t *Tokens) Macaroons() string {
	return strings.Join(t.macaroonTokens, ",")
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
