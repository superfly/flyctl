package sprites

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
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
	return sprites.CreateToken(ctx, flyMacaroon, org, "", apiURL)
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
		return "default"
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
			return err
		}

		if err := forgetToken(ctx, s); err != nil {
			return err
		}

		if s, err = newSession(ctx); err != nil {
			return err
		}
		defer s.Client.Close() //nolint:errcheck

		return fn(ctx, s)
	}
}

func isUnauthorized(err error) bool {
	if apiErr := sprites.IsAPIError(err); apiErr != nil {
		return apiErr.StatusCode == http.StatusUnauthorized
	}

	return strings.Contains(err.Error(), "status 401")
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

	s.Client = sprites.New(s.Token, sprites.WithBaseURL(s.APIURL), sprites.WithDisableControl())

	return s, nil
}

// resolveOrg picks the organization: the --org flag or FLY_ORG, then the
// .sprite file, then the saved default, then a prompt.
func resolveOrg(ctx context.Context) (string, error) {
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

	if os.Getenv(tokenEnv) != "" {
		// The token already identifies an organization.
		return "", nil
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
	st.DefaultOrg = org.RawSlug
	if st.DefaultOrg == "" {
		st.DefaultOrg = org.Slug
	}

	if err := spritestate.Save(configDir, st); err != nil {
		return "", fmt.Errorf("failed to save the default organization: %w", err)
	}

	return st.DefaultOrg, nil
}

// canonicalOrg turns the "personal" alias into the organization's real slug,
// which is what the Sprites API and the .sprite file use.
func canonicalOrg(ctx context.Context, slug string) (string, error) {
	if slug != "personal" || os.Getenv(tokenEnv) != "" {
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
func resolveToken(ctx context.Context, apiURL, org string) (token string, cached bool, err error) {
	if token := os.Getenv(tokenEnv); token != "" {
		return token, false, nil
	}

	configDir := state.ConfigDirectory(ctx)

	st, err := spritestate.Load(configDir)
	if err != nil {
		return "", false, err
	}

	key := spritestate.TokenKey(apiURL, org)
	if token := st.Tokens[key]; token != "" {
		return token, true, nil
	}

	toks := config.Tokens(ctx)
	if toks == nil || len(toks.GetMacaroonTokens()) == 0 {
		return "", false, flyerr.WithSuggestion(
			errors.New("sprites commands need a Fly.io login token"),
			"Run 'fly auth login', or set "+tokenEnv+" to a Sprites API token.",
		)
	}

	token, err = createToken(ctx, toks.Flaps(), org, apiURL)
	if err != nil {
		return "", false, fmt.Errorf("failed to get a Sprites token for organization %s: %w", org, err)
	}

	if st.Tokens == nil {
		st.Tokens = map[string]string{}
	}
	st.Tokens[key] = token

	if err := spritestate.Save(configDir, st); err != nil {
		return "", false, fmt.Errorf("failed to save the Sprites token: %w", err)
	}

	return token, false, nil
}

// forgetToken drops the saved token for the session's organization.
func forgetToken(ctx context.Context, s *session) error {
	configDir := state.ConfigDirectory(ctx)

	st, err := spritestate.Load(configDir)
	if err != nil {
		return err
	}

	delete(st.Tokens, spritestate.TokenKey(s.APIURL, s.Org))

	return spritestate.Save(configDir, st)
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
