package sprites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/spf13/cobra"
	"github.com/superfly/fly-go/tokens"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flag/flagnames"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/spritestate"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

const (
	defaultAPIURL = "https://api.sprites.dev"

	// tokenEnv supplies a Sprites API token directly, bypassing the Fly.io
	// login. The standalone sprite CLI reads the same variable.
	tokenEnv = "SPRITE_TOKEN"

	spriteFlagName = "sprite"
)

// createToken exchanges a Fly.io macaroon for a Sprites API token. It is a
// variable so tests can replace it.
var createToken = func(ctx context.Context, flyMacaroon, org, apiURL string) (string, error) {
	return spritestate.MintToken(ctx, apiURL, org, flyMacaroon)
}

// apiURL returns the Sprites API endpoint, honoring the environment variables
// the standalone sprite CLI uses.
func apiURL() string {
	for _, name := range []string{"SPRITES_API_URL", "SPRITE_URL"} {
		if v := strings.TrimRight(os.Getenv(name), "/"); v != "" {
			return v
		}
	}

	return defaultAPIURL
}

// session is everything a sprites command needs to talk to the Sprites API.
type session struct {
	// Org is the organization slug. It is empty when a token from
	// SPRITE_TOKEN is used and no organization was named.
	Org    string
	APIURL string
	Token  string
	Client *sprites.Client

	// cached reports whether Token came from the state file, in which case a
	// rejected token is worth replacing.
	cached bool
}

// orgDisplay names the organization in messages.
func (s *session) orgDisplay() string {
	if s.Org == "" {
		return "of the " + tokenEnv + " token"
	}

	return s.Org
}

// runner is a sprites command body.
type runner func(ctx context.Context, s *session) error

// newCommand builds a sprites subcommand that takes the shared --org and
// --sprite flags and runs with a session.
func newCommand(usage, short, long string, fn runner) *cobra.Command {
	cmd := command.New(usage, short, long, withSession(fn), requireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.String{
			Name:        spriteFlagName,
			Shorthand:   "s",
			Description: "The target sprite. Defaults to the one set with 'fly sprites use'",
		},
	)

	return cmd
}

// requireSession requires a Fly.io login unless a Sprites token is supplied
// through the environment.
func requireSession(ctx context.Context) (context.Context, error) {
	if os.Getenv(tokenEnv) != "" {
		return ctx, nil
	}

	return command.RequireSession(ctx)
}

// withSession runs fn with a session. A saved token that the API rejects is
// replaced once, since tokens can be revoked from the dashboard.
func withSession(fn runner) command.Runner {
	return func(ctx context.Context) error {
		s, err := newSession(ctx)
		if err != nil {
			return err
		}

		err = fn(ctx, s)
		s.Client.Close() //nolint:errcheck

		if err == nil || !s.cached || !isUnauthorized(err) {
			return explainLimit(err)
		}

		if err := forgetToken(ctx, s); err != nil {
			return err
		}

		if s, err = newSession(ctx); err != nil {
			return err
		}
		defer s.Client.Close() //nolint:errcheck

		return explainLimit(fn(ctx, s))
	}
}

// explainLimit adds what the API said about a rate or concurrency limit, and
// what to do about it, to an error caused by one.
func explainLimit(err error) error {
	var apiErr *sprites.APIError
	if err == nil || !errors.As(err, &apiErr) || !apiErr.IsRateLimitError() {
		return err
	}

	var advice []string

	switch {
	case apiErr.IsCreationRateLimited():
		if apiErr.Limit > 0 && apiErr.WindowSeconds > 0 {
			advice = append(advice, fmt.Sprintf("The limit is %d sprites per %ds.", apiErr.Limit, apiErr.WindowSeconds))
		}
	case apiErr.IsConcurrentLimitExceeded():
		if apiErr.Limit > 0 && apiErr.CurrentCount > 0 {
			advice = append(advice, fmt.Sprintf("%d of %d concurrent sprites are in use.", apiErr.CurrentCount, apiErr.Limit))
		}

		advice = append(advice, "Run 'fly sprites list' to see them and 'fly sprites destroy <name>' to free one up.")
	}

	if retryAfter := apiErr.GetRetryAfterSeconds(); retryAfter > 0 {
		advice = append(advice, fmt.Sprintf("Try again in %ds.", retryAfter))
	}

	if apiErr.UpgradeURL != "" {
		advice = append(advice, "To raise the limit, see "+apiErr.UpgradeURL)
	}

	return flyerr.WithSuggestion(err, strings.Join(advice, " "))
}

// isUnauthorized reports whether err is the API rejecting the token. The SDK
// reports that as an APIError from most calls, but as plain text from the
// filesystem and WebSocket ones.
func isUnauthorized(err error) bool {
	var apiErr *sprites.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusUnauthorized
	}

	msg := err.Error()

	return strings.Contains(msg, "status 401") ||
		strings.Contains(msg, "HTTP 401") ||
		strings.Contains(msg, "401 Unauthorized")
}

func newSession(ctx context.Context) (*session, error) {
	s := &session{APIURL: apiURL()}

	org, err := resolveOrg(ctx)
	if err != nil {
		return nil, err
	}
	s.Org = org

	if s.Token, s.cached, err = resolveToken(ctx, s.APIURL, s.Org); err != nil {
		return nil, err
	}

	s.Client = sprites.New(s.Token,
		sprites.WithBaseURL(s.APIURL),
		sprites.WithDisableControl(),
		sprites.WithHTTPClient(sessionHTTPClient(ctx)),
	)

	return s, nil
}

// resolveOrg picks the organization: the --org flag or FLY_ORG, then the
// .sprite file, then the saved default, then a prompt. None of that applies
// with a token from the environment.
func resolveOrg(ctx context.Context) (string, error) {
	// A token from the environment works in its own organization whatever
	// else names one, so no other organization is reported or recorded.
	if os.Getenv(tokenEnv) != "" {
		if slug := flag.GetOrg(ctx); slug != "" {
			fmt.Fprintf(iostreams.FromContext(ctx).ErrOut,
				"Warning: --org %s is ignored because %s is set; the token decides the organization.\n", slug, tokenEnv)
		}

		return "", nil
	}

	if slug := flag.GetOrg(ctx); slug != "" {
		return canonicalOrg(ctx, slug)
	}

	if sf, _, err := readSpriteFile(state.WorkingDirectory(ctx)); err != nil {
		return "", err
	} else if sf != nil && sf.Organization != "" {
		return sf.Organization, nil
	}

	configDir := state.ConfigDirectory(ctx)

	st, err := spritestate.Load(configDir)
	if err != nil {
		return "", err
	}

	if st.DefaultOrg != "" {
		return st.DefaultOrg, nil
	}

	org, err := prompt.Org(ctx)
	if err != nil {
		if prompt.IsNonInteractive(err) {
			return "", flyerr.WithSuggestion(
				errors.New("no organization specified"),
				"Pass --org, or run 'fly sprites use <sprite> --org <org>' to set one for this directory.",
			)
		}

		return "", err
	}

	// Remember the choice so the prompt appears once, not on every command.
	slug := org.RawSlug
	if slug == "" {
		slug = org.Slug
	}

	unlock, err := spritestate.LockMint(ctx, configDir)
	if err != nil {
		return "", err
	}
	defer unlock()

	if !hasExternalFlyToken(ctx) {
		if err := checkSavedLogin(ctx); err != nil {
			return "", err
		}
	}

	err = spritestate.Update(ctx, configDir, func(st *spritestate.State) { st.DefaultOrg = slug })
	if err != nil {
		return "", fmt.Errorf("failed to save the default organization: %w", err)
	}

	return slug, nil
}

// canonicalOrg turns the "personal" alias into the organization's real slug,
// which is what the Sprites API and the .sprite file use.
func canonicalOrg(ctx context.Context, slug string) (string, error) {
	if slug != "personal" {
		return slug, nil
	}

	org, err := orgs.OrgFromSlug(ctx, slug)
	if err != nil {
		return "", err
	}

	if org.RawSlug != "" {
		return org.RawSlug, nil
	}

	return org.Slug, nil
}

// resolveToken returns a Sprites API token for org: from the environment,
// from the state file, or freshly minted from the Fly.io login. Minted tokens
// are saved, because each mint creates a lasting token in the organization.
// The exception is a Fly.io token supplied just for this run.
func resolveToken(ctx context.Context, apiURL, org string) (token string, cached bool, err error) {
	if token := os.Getenv(tokenEnv); token != "" {
		return token, false, nil
	}

	toks := config.Tokens(ctx)
	if toks == nil {
		toks = tokens.Parse("")
	}

	if len(toks.GetMacaroonTokens()) == 0 {
		return "", false, flyerr.WithSuggestion(
			errors.New("sprites commands need a Fly.io login token"),
			"Run 'fly auth login', or set "+tokenEnv+" to a Sprites API token.",
		)
	}

	// Saved tokens were minted from the saved login. A Fly.io token supplied
	// for this run, with --access-token or the environment, may be another
	// user's or a deliberately limited one, so it neither reads nor adds to
	// them: it is exchanged afresh each time.
	saved := !hasExternalFlyToken(ctx)

	configDir := state.ConfigDirectory(ctx)

	key := spritestate.TokenKey(apiURL, org)

	if saved {
		// Login and logout take the same lock. Validate the prepared login
		// after acquiring it, before reading or writing any cached token.
		unlock, err := spritestate.LockMint(ctx, configDir)
		if err != nil {
			return "", false, err
		}
		defer unlock()

		if err := checkSavedLogin(ctx); err != nil {
			return "", false, err
		}

		st, err := spritestate.Load(configDir)
		if err != nil {
			return "", false, err
		}

		if token := st.Tokens[key]; token != "" {
			return token, true, nil
		}
	}

	token, err = createToken(ctx, toks.Flaps(), org, apiURL)
	if err != nil {
		return "", false, fmt.Errorf("failed to get a Sprites token for organization %s: %w", org, err)
	}

	if !saved {
		return token, false, nil
	}

	err = spritestate.Update(ctx, configDir, func(st *spritestate.State) {
		if st.Tokens == nil {
			st.Tokens = map[string]string{}
		}
		st.Tokens[key] = token
	})
	if err != nil {
		return "", false, fmt.Errorf("failed to save the Sprites token: %w", err)
	}

	return token, false, nil
}

// hasExternalFlyToken reports whether the Fly.io token for this run was
// supplied with --access-token or the environment rather than by `fly auth
// login`.
func hasExternalFlyToken(ctx context.Context) bool {
	for _, key := range []string{config.AccessTokenEnvKey, config.APITokenEnvKey} {
		if os.Getenv(key) != "" {
			return true
		}
	}

	return flag.GetString(ctx, flagnames.AccessToken) != ""
}

// forgetToken drops the saved token for the session's organization.
func forgetToken(ctx context.Context, s *session) error {
	unlock, err := spritestate.LockMint(ctx, state.ConfigDirectory(ctx))
	if err != nil {
		return err
	}
	defer unlock()

	if err := checkSavedLogin(ctx); err != nil {
		return err
	}

	return spritestate.Update(ctx, state.ConfigDirectory(ctx), func(st *spritestate.State) {
		key := spritestate.TokenKey(s.APIURL, s.Org)
		if st.Tokens[key] == s.Token {
			delete(st.Tokens, key)
		}
	})
}

// checkSavedLogin must be called while holding LockMint. An older command
// must not inherit a newer login's cached credentials, or save its own after
// that login has cleared the cache. Discharge refreshes keep the OAuth token.
func checkSavedLogin(ctx context.Context) error {
	saved, err := config.ReadAccessToken(state.ConfigFile(ctx))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read the saved login: %w", err)
	}

	current := tokens.Parse(saved)
	prepared := config.Tokens(ctx)
	if prepared == nil {
		prepared = tokens.Parse("")
	}

	if len(current.GetUserTokens()) > 0 || len(prepared.GetUserTokens()) > 0 {
		current, prepared = current.UserTokenOnly(), prepared.UserTokenOnly()
	}

	// Equal also compares the file provenance, which differs between the
	// freshly parsed disk value and config.Load's ParseFromFile value.
	if current.All() != prepared.All() {
		return errors.New("the saved Fly.io login changed; run the sprites command again")
	}

	return nil
}

// sessionHTTPClient lets large transfers run until canceled. The SDK uses a
// background context for Stat, ReadDir and ReadFile, so those requests inherit
// the command's context here. Explicit contexts, including exec's interrupt
// grace period, retain their own lifetime.
func sessionHTTPClient(ctx context.Context) *http.Client {
	return &http.Client{Transport: &sessionTransport{ctx: ctx, base: cleanhttp.DefaultPooledTransport()}}
}

type sessionTransport struct {
	ctx  context.Context
	base http.RoundTripper
}

func (t *sessionTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Context().Done() == nil {
		req = req.Clone(t.ctx)
	}

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// The SDK's filesystem drops status codes from structured error bodies.
	// Preserve this one before it does, so withSession can replace the token.
	if resp.StatusCode == http.StatusUnauthorized {
		_ = resp.Body.Close()

		return nil, &sprites.APIError{StatusCode: resp.StatusCode, Message: "authentication failed"}
	}

	return resp, nil
}

// errNoSprite is returned when nothing names the sprite to act on.
var errNoSprite = errors.New("no sprite specified")

// namedSprite resolves the sprite a command acts on without prompting. arg is
// the command's optional positional sprite name. Without one it falls back to
// the --sprite flag, then the .sprite file.
func (s *session) namedSprite(ctx context.Context, arg string) (string, error) {
	flagName := flag.GetString(ctx, spriteFlagName)

	switch {
	case arg != "" && flagName != "" && arg != flagName:
		return "", fmt.Errorf("sprite name specified both as an argument (%s) and with --sprite (%s)", arg, flagName)
	case arg != "":
		return arg, nil
	case flagName != "":
		return flagName, nil
	}

	sf, _, err := readSpriteFile(state.WorkingDirectory(ctx))
	if err != nil {
		return "", err
	}

	// A .sprite file names a sprite in one organization. It does not apply
	// when the command targets a different one.
	if sf != nil && sf.Sprite != "" && (sf.Organization == "" || s.Org == "" || sf.Organization == s.Org) {
		return sf.Sprite, nil
	}

	return "", flyerr.WithSuggestion(errNoSprite,
		"Pass --sprite, or run 'fly sprites use <sprite>' to set one for this directory.")
}

// spriteName is namedSprite, except that it offers a list to pick from when
// nothing names a sprite and the command is running interactively.
func (s *session) spriteName(ctx context.Context, arg string) (string, error) {
	name, err := s.namedSprite(ctx, arg)
	if !errors.Is(err, errNoSprite) || !iostreams.FromContext(ctx).IsInteractive() {
		return name, err
	}

	return s.selectSprite(ctx)
}

// sprite resolves the target sprite and returns a handle to it.
func (s *session) sprite(ctx context.Context, arg string) (*sprites.Sprite, error) {
	name, err := s.spriteName(ctx, arg)
	if err != nil {
		return nil, err
	}

	return s.Client.Sprite(name), nil
}

// selectSprite prompts for one of the organization's sprites.
func (s *session) selectSprite(ctx context.Context) (string, error) {
	if !iostreams.FromContext(ctx).IsInteractive() {
		return "", prompt.NonInteractiveError("a sprite must be specified when not running interactively")
	}

	result, err := s.Client.ListAllSpritesResult(ctx, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to list sprites: %w", err)
	}

	if len(result.Sprites) == 0 {
		return "", flyerr.WithSuggestion(
			fmt.Errorf("no sprites found in organization %s", s.orgDisplay()),
			"Create one with 'fly sprites create <name>'.",
		)
	}

	options := make([]string, len(result.Sprites))
	for i, sp := range result.Sprites {
		options[i] = fmt.Sprintf("%s (%s)", sp.Name(), sp.Status)
	}

	var index int
	if err := prompt.Select(ctx, &index, "Select a sprite:", "", options...); err != nil {
		return "", err
	}

	return result.Sprites[index].Name(), nil
}

// request sends an authenticated request to the Sprites API for the few
// endpoints the SDK does not wrap.
func (s *session) request(ctx context.Context, method, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.APIURL+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+s.Token)

	return http.DefaultClient.Do(req)
}
