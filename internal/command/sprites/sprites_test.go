package sprites

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/spritestate"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

// fakeAPI is a stand-in for the Sprites API that records the requests it gets.
type fakeAPI struct {
	*httptest.Server

	mu       sync.Mutex
	requests []string
	bodies   []string
}

func newFakeAPI(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *fakeAPI {
	t.Helper()

	api := &fakeAPI{}
	api.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		api.mu.Lock()
		api.requests = append(api.requests, r.Method+" "+r.URL.RequestURI())
		api.bodies = append(api.bodies, string(body))
		api.mu.Unlock()

		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		handler(w, r)
	}))
	t.Cleanup(api.Close)

	return api
}

// testEnv is the context a sprites command runs in during a test.
type testEnv struct {
	ctx       context.Context
	out       *bytes.Buffer
	errOut    *bytes.Buffer
	workDir   string
	configDir string
}

// newEnv builds a non-interactive command context for cmd with args parsed.
func newEnv(t *testing.T, cmd *cobra.Command, args ...string) *testEnv {
	t.Helper()

	// Keep the developer's own environment out of the tests.
	for _, name := range []string{tokenEnv, "SPRITES_API_URL", "SPRITE_URL", "FLY_ORG", config.AccessTokenEnvKey, config.APITokenEnvKey} {
		t.Setenv(name, "")
	}

	require.NoError(t, cmd.ParseFlags(args))

	ios, _, out, errOut := iostreams.Test()

	e := &testEnv{out: out, errOut: errOut, workDir: t.TempDir(), configDir: t.TempDir()}

	ctx := context.Background()
	ctx = iostreams.NewContext(ctx, ios)
	ctx = flag.NewContext(ctx, cmd.Flags())
	ctx = state.WithWorkingDirectory(ctx, e.workDir)
	ctx = state.WithConfigDirectory(ctx, e.configDir)
	ctx = config.NewContext(ctx, &config.Config{Tokens: tokens.Parse("fm2_test-macaroon")})
	e.ctx = ctx

	return e
}

func testSession(api *fakeAPI) *session {
	return &session{
		Org:    "acme",
		APIURL: api.URL,
		Token:  "test-token",
		Client: sprites.New("test-token", sprites.WithBaseURL(api.URL), sprites.WithDisableControl()),
	}
}

func writeJSONResponse(w http.ResponseWriter, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

func TestSpriteFile(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	sf, path, err := readSpriteFile(nested)
	require.NoError(t, err)
	assert.Nil(t, sf)
	assert.Empty(t, path)

	require.NoError(t, writeSpriteFile(root, "acme", "dev"))

	// The file is found from a subdirectory.
	sf, path, err = readSpriteFile(nested)
	require.NoError(t, err)
	assert.Equal(t, &spriteFile{Organization: "acme", Sprite: "dev"}, sf)
	assert.Equal(t, filepath.Join(root, ".sprite"), path)

	// The format is the one the standalone sprite CLI writes.
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.JSONEq(t, `{"organization":"acme","sprite":"dev"}`, string(data))

	require.NoError(t, removeSpriteFile(root))

	sf, _, err = readSpriteFile(nested)
	require.NoError(t, err)
	assert.Nil(t, sf)
}

func TestSpriteFileDirectoryForm(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".sprite"), 0o755))

	// An empty .sprite directory selects nothing.
	sf, _, err := readSpriteFile(root)
	require.NoError(t, err)
	assert.Nil(t, sf)

	require.NoError(t, writeSpriteFile(root, "acme", "dev"))

	sf, path, err := readSpriteFile(root)
	require.NoError(t, err)
	assert.Equal(t, "dev", sf.Sprite)
	assert.Equal(t, filepath.Join(root, ".sprite", "selected"), path)
}

func TestAPIURL(t *testing.T) {
	t.Setenv("SPRITES_API_URL", "")
	t.Setenv("SPRITE_URL", "")
	assert.Equal(t, defaultAPIURL, apiURL())

	t.Setenv("SPRITE_URL", "https://legacy.example/")
	assert.Equal(t, "https://legacy.example", apiURL())

	t.Setenv("SPRITES_API_URL", "https://api.example")
	assert.Equal(t, "https://api.example", apiURL())
}

func TestResolveTokenMintsOnceThenUsesCache(t *testing.T) {
	e := newEnv(t, newList())

	var mints int

	original := createToken
	t.Cleanup(func() { createToken = original })

	createToken = func(_ context.Context, flyMacaroon, org, apiURL string) (string, error) {
		mints++

		assert.Equal(t, "fm2_test-macaroon", flyMacaroon)
		assert.Equal(t, "https://api.example", apiURL)

		return "minted-for-" + org, nil
	}

	token, cached, err := resolveToken(e.ctx, "https://api.example", "acme")
	require.NoError(t, err)
	assert.Equal(t, "minted-for-acme", token)
	assert.False(t, cached)

	token, cached, err = resolveToken(e.ctx, "https://api.example", "acme")
	require.NoError(t, err)
	assert.Equal(t, "minted-for-acme", token)
	assert.True(t, cached)

	assert.Equal(t, 1, mints, "a token must be minted once and then reused")

	// A different organization gets its own token.
	token, _, err = resolveToken(e.ctx, "https://api.example", "other")
	require.NoError(t, err)
	assert.Equal(t, "minted-for-other", token)
	assert.Equal(t, 2, mints)
}

func TestResolveTokenPrefersEnvironment(t *testing.T) {
	e := newEnv(t, newList())
	t.Setenv(tokenEnv, "from-env")

	original := createToken
	t.Cleanup(func() { createToken = original })

	createToken = func(context.Context, string, string, string) (string, error) {
		t.Fatal("no token should be minted when SPRITE_TOKEN is set")
		return "", nil
	}

	token, cached, err := resolveToken(e.ctx, defaultAPIURL, "acme")
	require.NoError(t, err)
	assert.Equal(t, "from-env", token)
	assert.False(t, cached)

	assert.NoFileExists(t, spritestate.Path(e.configDir), "an environment token must not be saved")
}

func TestResolveTokenIgnoresCacheForExternalFlyToken(t *testing.T) {
	e := newEnv(t, newList())

	key := spritestate.TokenKey(defaultAPIURL, "acme")
	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{
		Tokens: map[string]string{key: "from-saved-login"},
	}))

	// A Fly.io token supplied for this run may be a different user's.
	t.Setenv(config.APITokenEnvKey, "fm2_someone-else")

	original := createToken
	t.Cleanup(func() { createToken = original })

	mints := 0
	createToken = func(context.Context, string, string, string) (string, error) {
		mints++
		return "from-external", nil
	}

	for range 2 {
		token, cached, err := resolveToken(e.ctx, defaultAPIURL, "acme")
		require.NoError(t, err)
		assert.Equal(t, "from-external", token)
		assert.False(t, cached)
	}

	assert.Equal(t, 2, mints, "an external token is exchanged on every run")

	// The saved login's token is neither used nor replaced.
	st, err := spritestate.Load(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, "from-saved-login", st.Tokens[key])
}

func TestResolveTokenNeedsMacaroon(t *testing.T) {
	e := newEnv(t, newList())
	e.ctx = config.NewContext(e.ctx, &config.Config{Tokens: tokens.Parse("")})

	_, _, err := resolveToken(e.ctx, defaultAPIURL, "acme")
	require.Error(t, err)
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "fly auth login")
}

func TestResolveOrgPrecedence(t *testing.T) {
	// Saved default, when nothing else names an organization.
	e := newEnv(t, newList())
	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{DefaultOrg: "saved"}))

	org, err := resolveOrg(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, "saved", org)

	// The .sprite file beats the saved default.
	require.NoError(t, writeSpriteFile(e.workDir, "from-file", "dev"))

	org, err = resolveOrg(e.ctx)
	require.NoError(t, err)
	assert.Equal(t, "from-file", org)

	// The flag beats both.
	e2 := newEnv(t, newList(), "--org", "from-flag")
	require.NoError(t, writeSpriteFile(e2.workDir, "from-file", "dev"))

	org, err = resolveOrg(e2.ctx)
	require.NoError(t, err)
	assert.Equal(t, "from-flag", org)
}

func TestResolveOrgWithEnvTokenNeedsNoOrg(t *testing.T) {
	e := newEnv(t, newList())
	t.Setenv(tokenEnv, "from-env")

	// The saved login's default organization is not the token's.
	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{DefaultOrg: "saved"}))

	org, err := resolveOrg(e.ctx)
	require.NoError(t, err)
	assert.Empty(t, org)
}

func TestNamedSprite(t *testing.T) {
	s := &session{Org: "acme"}

	e := newEnv(t, newInfo())
	_, err := s.namedSprite(e.ctx, "")
	require.ErrorIs(t, err, errNoSprite)

	name, err := s.namedSprite(e.ctx, "from-arg")
	require.NoError(t, err)
	assert.Equal(t, "from-arg", name)

	require.NoError(t, writeSpriteFile(e.workDir, "acme", "from-file"))
	name, err = s.namedSprite(e.ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "from-file", name)

	// A .sprite file for another organization does not apply.
	require.NoError(t, writeSpriteFile(e.workDir, "other", "from-file"))
	_, err = s.namedSprite(e.ctx, "")
	require.ErrorIs(t, err, errNoSprite)

	e = newEnv(t, newInfo(), "--sprite", "from-flag")
	name, err = s.namedSprite(e.ctx, "")
	require.NoError(t, err)
	assert.Equal(t, "from-flag", name)

	// The same name given twice is fine; two different names are not.
	_, err = s.namedSprite(e.ctx, "from-flag")
	require.NoError(t, err)

	_, err = s.namedSprite(e.ctx, "different")
	require.ErrorContains(t, err, "both as an argument")
}

func TestWithSessionReplacesRejectedSavedToken(t *testing.T) {
	e := newEnv(t, newList(), "--org", "acme")
	t.Setenv("SPRITES_API_URL", "https://api.example")

	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{
		Tokens: map[string]string{spritestate.TokenKey("https://api.example", "acme"): "revoked"},
	}))

	original := createToken
	t.Cleanup(func() { createToken = original })

	createToken = func(context.Context, string, string, string) (string, error) {
		return "fresh", nil
	}

	var seen []string

	err := withSession(func(_ context.Context, s *session) error {
		seen = append(seen, s.Token)

		if s.Token == "revoked" {
			return &sprites.APIError{StatusCode: http.StatusUnauthorized, Message: "unauthorized"}
		}

		return nil
	})(e.ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"revoked", "fresh"}, seen)

	st, err := spritestate.Load(e.configDir)
	require.NoError(t, err)
	assert.Equal(t, "fresh", st.Tokens[spritestate.TokenKey("https://api.example", "acme")])
}

func TestWithSessionDoesNotRetryOtherErrors(t *testing.T) {
	e := newEnv(t, newList(), "--org", "acme")

	require.NoError(t, spritestate.Save(e.configDir, &spritestate.State{
		Tokens: map[string]string{spritestate.TokenKey(defaultAPIURL, "acme"): "saved"},
	}))

	calls := 0
	boom := errors.New("boom")

	err := withSession(func(context.Context, *session) error {
		calls++
		return boom
	})(e.ctx)

	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, calls)
}

var listResponse = map[string]any{
	"sprites": []map[string]any{
		{"id": "sprite-1", "name": "alpha", "organization": "acme", "status": "running", "url": "https://alpha.sprites.app"},
		{"id": "sprite-2", "name": "beta", "organization": "acme", "status": "cold", "url_settings": map[string]any{"auth": "public"}},
	},
	"org": map[string]any{"name": "acme", "running": 1, "warm": 0, "cold": 1, "running_limit": 10, "warm_limit": 10},
}

func TestListPipedPrintsNames(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, listResponse)
	})

	e := newEnv(t, newList(), "--prefix", "a")
	require.NoError(t, runList(e.ctx, testSession(api)))

	assert.Equal(t, "alpha\nbeta\n", e.out.String())
	require.Len(t, api.requests, 1)
	assert.Contains(t, api.requests[0], "prefix=a")
}

func TestListJSON(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, listResponse)
	})

	e := newEnv(t, newList(), "--json")
	require.NoError(t, runList(e.ctx, testSession(api)))

	var got struct {
		Sprites []struct {
			Name        string `json:"name"`
			Status      string `json:"status"`
			URLSettings *struct {
				Auth string `json:"auth"`
			} `json:"url_settings"`
		} `json:"sprites"`
		Org struct {
			Name         string `json:"name"`
			RunningLimit int    `json:"running_limit"`
		} `json:"org"`
	}
	require.NoError(t, json.Unmarshal(e.out.Bytes(), &got))

	require.Len(t, got.Sprites, 2)
	assert.Equal(t, "alpha", got.Sprites[0].Name)
	assert.Equal(t, "cold", got.Sprites[1].Status)
	assert.Equal(t, "public", got.Sprites[1].URLSettings.Auth)
	assert.Equal(t, "acme", got.Org.Name)
	assert.Equal(t, 10, got.Org.RunningLimit)
}

func TestListRejectsJSONWithWatch(t *testing.T) {
	e := newEnv(t, newList(), "--json", "--watch")
	require.ErrorContains(t, runList(e.ctx, &session{}), "cannot be combined")
}

func TestConfigUpdateValidation(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "at least one of"},
		{[]string{"--url-auth", "nope"}, `invalid --url-auth "nope"`},
		{[]string{"--private-access", "everyone"}, `invalid --private-access "everyone"`},
		{[]string{"--url-auth", "public", "--private-access", "admins"}, "only applies when"},
	}

	for _, tt := range tests {
		e := newEnv(t, newConfigUpdate(), tt.args...)
		require.ErrorContains(t, runConfigUpdate(e.ctx, &session{}), tt.want)
	}
}

func TestConfigUpdateSendsRequest(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, map[string]any{"name": "dev"})
	})

	e := newEnv(t, newConfigUpdate(), "--sprite", "dev", "--url-auth", "public", "--label", "env:prod", "--clear-labels")
	require.NoError(t, runConfigUpdate(e.ctx, testSession(api)))

	require.Len(t, api.requests, 1)
	assert.Equal(t, "PUT /v1/sprites/dev", api.requests[0])
	assert.JSONEq(t, `{"url_settings":{"auth":"public"},"labels":["env:prod"],"clear_labels":true}`, api.bodies[0])
	assert.Equal(t, "Updated dev\n", e.out.String())
}

func TestDestroyNeedsConfirmationWhenNotInteractive(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL)
	})

	e := newEnv(t, newDestroy(), "dev")
	err := runDestroy(e.ctx, testSession(api))
	require.ErrorContains(t, err, "needs confirmation")
	assert.Empty(t, api.requests)
}

func TestDestroyRemovesMatchingSpriteFile(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	e := newEnv(t, newDestroy(), "--yes")
	require.NoError(t, writeSpriteFile(e.workDir, "acme", "dev"))

	require.NoError(t, runDestroy(e.ctx, testSession(api)))

	assert.Equal(t, []string{"DELETE /v1/sprites/dev"}, api.requests)
	assert.NoFileExists(t, filepath.Join(e.workDir, ".sprite"))
}

func TestDestroyKeepsSpriteFileForAnotherSprite(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	e := newEnv(t, newDestroy(), "--yes", "other")
	require.NoError(t, writeSpriteFile(e.workDir, "acme", "dev"))

	require.NoError(t, runDestroy(e.ctx, testSession(api)))
	assert.FileExists(t, filepath.Join(e.workDir, ".sprite"))
}

func TestCheckpointDeleteStatuses(t *testing.T) {
	tests := []struct {
		status  int
		wantErr string
	}{
		{http.StatusNoContent, ""},
		{http.StatusNotFound, "checkpoint v3 not found"},
		{http.StatusConflict, "active checkpoint"},
		{http.StatusInternalServerError, "500"},
	}

	for _, tt := range tests {
		api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tt.status)
		})

		e := newEnv(t, newCheckpointDelete(), "--sprite", "dev", "v3")
		err := runCheckpointDelete(e.ctx, testSession(api))

		assert.Equal(t, []string{"DELETE /v1/sprites/dev/checkpoints/v3"}, api.requests)

		if tt.wantErr == "" {
			require.NoError(t, err)
			assert.Contains(t, e.out.String(), "Checkpoint v3 deleted")
		} else {
			require.ErrorContains(t, err, tt.wantErr)
		}
	}
}

// scriptedStream replays checkpoint progress messages.
type scriptedStream struct {
	messages []*sprites.StreamMessage
}

func (s *scriptedStream) Next() (*sprites.StreamMessage, error) {
	if len(s.messages) == 0 {
		return nil, io.EOF
	}

	msg := s.messages[0]
	s.messages = s.messages[1:]

	return msg, nil
}

func (s *scriptedStream) Close() error { return nil }

func TestConsumeProgress(t *testing.T) {
	var seen []string
	record := func(msg *sprites.StreamMessage) { seen = append(seen, msg.Type) }

	err := consumeProgress(&scriptedStream{messages: []*sprites.StreamMessage{
		{Type: "info", Data: "  ID: v2"},
		{Type: "complete"},
	}}, "checkpoint", record)
	require.NoError(t, err)
	assert.Equal(t, []string{"info"}, seen)

	err = consumeProgress(&scriptedStream{messages: []*sprites.StreamMessage{
		{Type: "error", Error: "disk full"},
	}}, "checkpoint", record)
	require.ErrorContains(t, err, "disk full")

	// A stream that ends without "complete" is not a success.
	err = consumeProgress(&scriptedStream{messages: []*sprites.StreamMessage{
		{Type: "info", Data: "working"},
	}}, "restore", record)
	require.ErrorContains(t, err, "before the restore was confirmed")
}

func TestSessionsKill(t *testing.T) {
	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"type":"signal","message":"Signaling TERM"}`+"\n")
		io.WriteString(w, `{"type":"complete","exit_code":143}`+"\n")
	})

	e := newEnv(t, newSessionsKill(), "--sprite", "dev", "--signal", "sigint", "--timeout", "3s", "42")
	require.NoError(t, runSessionsKill(e.ctx, testSession(api)))

	assert.Equal(t, []string{"POST /v1/sprites/dev/exec/42/kill?signal=INT&timeout=3s"}, api.requests)
	assert.Equal(t, "Signaling TERM\nSession ended with exit status 143\n", e.out.String())
}

func TestPrintKillProgressError(t *testing.T) {
	var out bytes.Buffer

	err := printKillProgress(&out, strings.NewReader(`{"type":"error","message":"session not found: 9"}`+"\n"))
	require.ErrorContains(t, err, "session not found: 9")
}

func TestPickSession(t *testing.T) {
	now := time.Now()
	sessions := []*sprites.Session{
		{ID: "1", Command: "bash -l", Created: now.Add(-time.Minute)},
		{ID: "2", Command: "bash", Created: now.Add(-time.Hour)},
		{ID: "bash", Command: "zsh", Created: now},
	}

	ctx := newEnv(t, newAttach()).ctx

	// An exact ID wins over a command match.
	got, err := pickSession(ctx, sessions, "bash")
	require.NoError(t, err)
	assert.Equal(t, "bash", got.ID)

	// Otherwise the oldest session running the command.
	got, err = pickSession(ctx, sessions[:2], "bash")
	require.NoError(t, err)
	assert.Equal(t, "2", got.ID)

	_, err = pickSession(ctx, sessions, "vim")
	require.ErrorContains(t, err, `matching "vim"`)

	// Nothing to pick with when not interactive.
	_, err = pickSession(ctx, sessions, "")
	require.ErrorContains(t, err, "required when not running interactively")
}

func TestExecFlagValidation(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "a command is required"},
		{[]string{"--stdin", "--no-stdin", "ls"}, "cannot be used together"},
		{[]string{"--stdin", "--tty", "ls"}, "--stdin is not supported with --tty"},
		{[]string{"--no-stdin", "--tty", "ls"}, "--no-stdin is not supported with --tty"},
		{[]string{"--file", "nodest", "ls"}, "expected source:dest"},
	}

	for _, tt := range tests {
		e := newEnv(t, newExec(), tt.args...)
		require.ErrorContains(t, runExec(e.ctx, &session{}), tt.want)
	}
}

func TestExecPassesCommandFlagsThrough(t *testing.T) {
	// Flags after the command name belong to the command, with or without --.
	e := newEnv(t, newExec(), "--dir", "/app", "ls", "-la", "--color")
	assert.Equal(t, []string{"ls", "-la", "--color"}, flag.Args(e.ctx))
	assert.Equal(t, "/app", flag.GetString(e.ctx, "dir"))

	e = newEnv(t, newExec(), "--", "ls", "-la")
	assert.Equal(t, []string{"ls", "-la"}, flag.Args(e.ctx))
}

func TestParseEnv(t *testing.T) {
	// Given once, pairs may be comma-separated.
	assert.Equal(t, []string{"A=1", "B=2"}, parseEnv([]string{"A=1, B=2"}))
	assert.Empty(t, parseEnv(nil))

	// A comma inside a value is kept: only ",KEY=" starts a new pair.
	assert.Equal(t, []string{"JAVA_OPTS=-Xmx1g,-Xms1g", "N=1"}, parseEnv([]string{"JAVA_OPTS=-Xmx1g,-Xms1g,N=1"}))

	// Given more than once, each value is literal.
	assert.Equal(t,
		[]string{"DSN=host=db,port=5432,user=app", "B=2"},
		parseEnv([]string{"DSN=host=db,port=5432,user=app", "B=2"}),
	)
}

func TestParseFileSpecs(t *testing.T) {
	got, err := parseFileSpecs([]string{"./a.txt:/tmp/a.txt", `C:\work\app.conf:/etc/app.conf`})
	require.NoError(t, err)
	assert.Equal(t, []fileSpec{
		{source: "./a.txt", dest: "/tmp/a.txt"},
		{source: `C:\work\app.conf`, dest: "/etc/app.conf"},
	}, got)

	for _, bad := range []string{"nodest", ":dest", "src:", `C:\only-a-source`} {
		_, err := parseFileSpecs([]string{bad})
		require.Error(t, err, bad)
	}
}

func TestIsTransient(t *testing.T) {
	for _, err := range []error{
		&sprites.APIError{StatusCode: http.StatusBadGateway},
		errors.New("websocket: bad handshake: status 503"),
		errors.New("dial tcp 1.2.3.4:443: connect: connection refused"),
	} {
		assert.True(t, isTransient(err), err.Error())
	}

	for _, err := range []error{
		&sprites.APIError{StatusCode: http.StatusNotFound},
		errors.New("failed to connect: HTTP 404"),
		context.Canceled,
		errors.New("no such command"),
	} {
		assert.False(t, isTransient(err), err.Error())
	}
}

func TestExplainLimit(t *testing.T) {
	err := explainLimit(fmt.Errorf("failed to create sprite: %w", &sprites.APIError{
		StatusCode:   http.StatusTooManyRequests,
		ErrorCode:    sprites.ErrCodeConcurrentLimitExceeded,
		Message:      "too many sprites",
		Limit:        10,
		CurrentCount: 10,
	}))

	assert.Contains(t, flyerr.GetErrorSuggestion(err), "10 of 10 concurrent sprites")
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "fly sprites destroy")

	plain := errors.New("boom")
	assert.Equal(t, plain, explainLimit(plain))
}

func TestIsUnauthorized(t *testing.T) {
	apiErr := &sprites.APIError{StatusCode: http.StatusUnauthorized, Message: "unauthorized"}

	// Commands wrap the SDK's errors, so the check must see through that.
	assert.True(t, isUnauthorized(fmt.Errorf("failed to list sprites: %w", apiErr)))
	assert.True(t, isUnauthorized(&fs.PathError{Op: "stat", Path: "/x", Err: errors.New("HTTP 401")}))

	assert.False(t, isUnauthorized(fmt.Errorf("failed: %w", &sprites.APIError{StatusCode: http.StatusNotFound})))
	assert.False(t, isUnauthorized(errors.New("boom")))
}

func TestFileEditRefusesEmptyStdin(t *testing.T) {
	var writes int

	api := newFakeAPI(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method != http.MethodGet:
			writes++
		case strings.HasPrefix(r.URL.Path, "/v1/sprites/dev/fs/list"):
			writeJSONResponse(w, map[string]any{
				"path":    "/etc/app.conf",
				"entries": []map[string]any{{"name": "app.conf", "path": "/etc/app.conf", "mode": "0644", "size": 3}},
				"count":   1,
			})
		default:
			_, _ = io.WriteString(w, "old")
		}
	})

	// iostreams.Test gives an empty, non-terminal stdin, like </dev/null.
	e := newEnv(t, newFileEdit(), "--sprite", "dev", "/etc/app.conf")
	err := runFileEdit(e.ctx, testSession(api))

	require.ErrorContains(t, err, "left unchanged")
	assert.Zero(t, writes, "the remote file must not be written")
}

func TestWithTerminalEnv(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("COLORTERM", "")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "")

	// An explicit value is not overridden by the local terminal's.
	got := withTerminalEnv([]string{"LANG=C", "FOO=bar"})
	assert.Equal(t, []string{"LANG=C", "FOO=bar", "TERM=xterm-256color"}, got)
}

func TestDetachReader(t *testing.T) {
	detached := make(chan struct{})
	r := &detachReader{r: strings.NewReader("ab\x1ccd"), detached: detached}

	buf := make([]byte, 16)
	n, err := r.Read(buf)

	// Input before the detach character is delivered, the rest is not.
	assert.Equal(t, "ab", string(buf[:n]))
	require.ErrorIs(t, err, io.EOF)

	select {
	case <-detached:
	default:
		t.Fatal("the detach character should signal a detach")
	}
}

func TestDetachReaderPassesOrdinaryInput(t *testing.T) {
	detached := make(chan struct{})
	r := &detachReader{r: strings.NewReader("plain"), detached: detached}

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "plain", string(data))

	select {
	case <-detached:
		t.Fatal("ordinary input must not detach")
	default:
	}
}

func TestParsePortMappings(t *testing.T) {
	got, err := parsePortMappings([]string{"8080", "3001:3000"})
	require.NoError(t, err)
	assert.Equal(t, []sprites.PortMapping{
		{LocalPort: 8080, RemotePort: 8080},
		{LocalPort: 3001, RemotePort: 3000},
	}, got)

	for _, bad := range []string{"abc", "0", "70000", "80:", ":80", "80:x"} {
		_, err := parsePortMappings([]string{bad})
		require.Error(t, err, bad)
	}
}

func TestProxyArgumentValidation(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "at least one port"},
		{[]string{"--ssh", "--stdio", ":22"}, "cannot be used together"},
		{[]string{"--ssh", "8080"}, "cannot be combined"},
		{[]string{"-W", ":22", "8080"}, "cannot be combined"},
		{[]string{"nope"}, "invalid port mapping"},
	}

	for _, tt := range tests {
		e := newEnv(t, newProxy(), tt.args...)
		require.ErrorContains(t, runProxy(e.ctx, &session{}), tt.want)
	}
}

func TestAPIPath(t *testing.T) {
	tests := []struct {
		path, sprite, want string
	}{
		{"/checkpoints", "dev", "/v1/sprites/dev/checkpoints"},
		{"checkpoints", "dev", "/v1/sprites/dev/checkpoints"},
		{"/v1/sprites", "dev", "/v1/sprites"},
		{"/v1/sprites/other/exec", "dev", "/v1/sprites/other/exec"},
		{"/sprites", "", "/v1/sprites"},
		{"/v1/sprites", "", "/v1/sprites"},
		{"/v1", "", "/v1"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.want, apiPath(tt.path, tt.sprite), "%s with sprite %q", tt.path, tt.sprite)
	}
}

func TestValidateSpriteName(t *testing.T) {
	for _, good := range []string{"abc", "my-sprite", "a1b", "0-0"} {
		require.NoError(t, validateSpriteName(good), good)
	}

	for _, bad := range []string{"", "ab", "-abc", "abc-", "My-Sprite", "under_score", strings.Repeat("a", 56)} {
		require.Error(t, validateSpriteName(bad), bad)
	}
}

func TestFormatAge(t *testing.T) {
	tests := map[time.Duration]string{
		30 * time.Second: "30s",
		5 * time.Minute:  "5m",
		3 * time.Hour:    "3h",
		30 * time.Hour:   "1 day",
		72 * time.Hour:   "3 days",
	}

	for d, want := range tests {
		assert.Equal(t, want, formatAge(d))
	}
}

func TestEnvNames(t *testing.T) {
	assert.Equal(t, "TERM,LANG", envNames([]string{"TERM=xterm", "LANG=C", "novalue", "=x"}))
	assert.Empty(t, envNames(nil))
}

func TestSSHHostKeyIsStablePerSprite(t *testing.T) {
	s := &session{APIURL: defaultAPIURL, Org: "acme"}

	a, err := sshHostKey(s, "dev")
	require.NoError(t, err)

	b, err := sshHostKey(s, "dev")
	require.NoError(t, err)

	other, err := sshHostKey(s, "prod")
	require.NoError(t, err)

	assert.Equal(t, a.PublicKey().Marshal(), b.PublicKey().Marshal())
	assert.NotEqual(t, a.PublicKey().Marshal(), other.PublicKey().Marshal())
}
