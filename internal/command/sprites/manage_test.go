package sprites

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/superfly/flyctl/internal/flyerr"
)

func TestCommandTree(t *testing.T) {
	root := New()

	want := []string{
		"api", "attach", "checkpoint", "config", "console", "create", "destroy", "exec",
		"file", "info", "list", "proxy", "restore", "sessions", "url", "use",
	}

	var got []string
	for _, cmd := range root.Commands() {
		got = append(got, cmd.Name())
	}

	assert.ElementsMatch(t, want, got)
	assert.Contains(t, root.Aliases, "sprite")

	// Every command says what it does, and every leaf takes the shared flags.
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		assert.NotEmpty(t, cmd.Short, cmd.CommandPath())
		assert.NotEmpty(t, cmd.Long, cmd.CommandPath())

		if cmd.Runnable() {
			assert.NotNil(t, cmd.Flags().Lookup("org"), cmd.CommandPath())
			assert.NotNil(t, cmd.Flags().Lookup("sprite"), cmd.CommandPath())
		}

		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)

	subcommands := func(name string) []string {
		var names []string

		for _, cmd := range root.Commands() {
			if cmd.Name() != name {
				continue
			}

			for _, child := range cmd.Commands() {
				names = append(names, child.Name())
			}
		}

		return names
	}

	assert.ElementsMatch(t, []string{"create", "list", "info", "delete", "restore"}, subcommands("checkpoint"))
	assert.ElementsMatch(t, []string{"list", "attach", "kill"}, subcommands("sessions"))
	assert.ElementsMatch(t, []string{"edit", "push", "pull"}, subcommands("file"))
	assert.ElementsMatch(t, []string{"update"}, subcommands("config"))
	assert.ElementsMatch(t, []string{"update"}, subcommands("url"))
}

func TestRequireSessionSkipsLoginWithEnvToken(t *testing.T) {
	t.Setenv(tokenEnv, "from-env")

	// With a token in the environment there is no Fly.io session to check,
	// so the context is returned untouched rather than examined.
	ctx := context.Background()

	got, err := requireSession(ctx)
	require.NoError(t, err)
	assert.Equal(t, ctx, got)
}

func TestNewSessionWithEnvToken(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newList())
	t.Setenv(tokenEnv, "test-token")
	t.Setenv("SPRITES_API_URL", api.URL)

	s, err := newSession(e.ctx)
	require.NoError(t, err)

	assert.Equal(t, api.URL, s.APIURL)
	assert.Equal(t, "test-token", s.Token)
	assert.Empty(t, s.Org)
	assert.False(t, s.cached)
	assert.Contains(t, s.orgDisplay(), tokenEnv)

	// The client it built reaches the API with that token.
	sprite, err := s.Client.GetSprite(e.ctx, "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev", sprite.Name())
}

func TestCanonicalOrgLeavesRealSlugsAlone(t *testing.T) {
	e := newEnv(t, newList())

	org, err := canonicalOrg(e.ctx, "acme")
	require.NoError(t, err)
	assert.Equal(t, "acme", org)
}

func TestSpriteNameNotInteractive(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newInfo())

	// Nothing names a sprite and nobody can be asked.
	_, err := api.session().spriteName(e.ctx, "")
	require.ErrorIs(t, err, errNoSprite)
	assert.Contains(t, flyerr.GetErrorSuggestion(err), "fly sprites use")

	_, err = api.session().selectSprite(e.ctx)
	require.ErrorContains(t, err, "not running interactively")
	assert.Empty(t, api.requested(), "nothing is listed when nobody can pick")

	sprite, err := api.session().sprite(e.ctx, "dev")
	require.NoError(t, err)
	assert.Equal(t, "dev", sprite.Name())
}

func TestCreate(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newCreate(), "--label", "env:dev", "--label", "team:web", "my-sprite")
	require.NoError(t, runCreate(e.ctx, api.session()))

	out := e.out.String()
	assert.Contains(t, out, "Creating sprite my-sprite in organization acme")
	assert.Contains(t, out, "Created sprite my-sprite")
	assert.Contains(t, out, "URL: https://my-sprite.sprites.app")

	// Not a terminal, so no console is opened.
	assert.NotContains(t, out, "Connecting to console")
	assert.Nil(t, api.lastExecQuery())

	assert.Equal(t, []string{"env:dev", "team:web"}, api.sprites["my-sprite"].info["labels"])
}

func TestCreateTakesNameFromSpriteFlag(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newCreate(), "--sprite", "from-flag")
	require.NoError(t, runCreate(e.ctx, api.session()))
	assert.Contains(t, api.sprites, "from-flag")
}

func TestCreateRejectsBadNames(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newCreate(), "Bad_Name")
	require.ErrorContains(t, runCreate(e.ctx, api.session()), "lowercase letters")

	// With no name and nobody to ask, say so rather than failing obscurely.
	e = newEnv(t, newCreate())
	require.ErrorContains(t, runCreate(e.ctx, api.session()), "sprite name is required")

	assert.Empty(t, api.requested(), "nothing is created from an invalid request")
}

func TestCreateReportsAPIError(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newCreate(), "my-sprite")
	require.ErrorContains(t, runCreate(e.ctx, sessionWithToken(api, "wrong")), "failed to create sprite")
	assert.NotContains(t, e.out.String(), "Created sprite")
}

func TestUse(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newUse(), "dev")
	require.NoError(t, runUse(e.ctx, api.session()))

	sf, _, err := readSpriteFile(e.workDir)
	require.NoError(t, err)
	assert.Equal(t, &spriteFile{Organization: "acme", Sprite: "dev"}, sf)
	assert.Contains(t, e.out.String(), "Now using sprite dev from organization acme")
}

func TestUseRejectsUnknownSprite(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newUse(), "ghost")
	require.ErrorContains(t, runUse(e.ctx, api.session()), "failed to find sprite ghost")

	assert.NoFileExists(t, filepath.Join(e.workDir, ".sprite"), "a sprite that does not exist is not recorded")
}

func TestUseNeedsNameWhenNotInteractive(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newUse())
	require.ErrorContains(t, runUse(e.ctx, api.session()), "sprite name is required")
}

func TestUseUnset(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	require.NoError(t, writeSpriteFile(dir, "acme", "dev"))

	// --unset works without a session: the command's own RunE handles it
	// before anything asks for a login.
	var out bytes.Buffer

	cmd := newUse()
	cmd.SetOut(&out)
	require.NoError(t, cmd.ParseFlags([]string{"--unset"}))
	require.NoError(t, cmd.RunE(cmd, nil))

	assert.NoFileExists(t, filepath.Join(dir, ".sprite"))
	assert.Contains(t, out.String(), "Removed the .sprite file")

	// A second time there is nothing to remove.
	require.ErrorContains(t, cmd.RunE(cmd, nil), "no .sprite file")
}

func TestInfo(t *testing.T) {
	api := newSpritesAPI(t)
	sp := api.addSprite("dev")
	sp.info["labels"] = []string{"env:dev"}
	sp.info["url_settings"] = map[string]any{"auth": "public"}
	sp.info["last_running_at"] = time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)

	e := newEnv(t, newInfo(), "dev")
	require.NoError(t, runInfo(e.ctx, api.session()))

	out := e.out.String()
	for _, want := range []string{
		"Name:            dev",
		"Organization:    acme",
		"ID:              sprite-dev",
		"Status:          running",
		"Created:         2026-01-02T03:04:05Z",
		"Last running:    2026-02-03T04:05:06Z",
		"URL:             https://dev.sprites.app",
		"Auth:            public",
		"Labels:          env:dev",
	} {
		assert.Contains(t, out, want)
	}
}

func TestInfoJSON(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev").info["labels"] = []string{"env:dev"}

	e := newEnv(t, newInfo(), "--json", "dev")
	require.NoError(t, runInfo(e.ctx, api.session()))

	var got spriteJSON
	require.NoError(t, json.Unmarshal(e.out.Bytes(), &got))
	assert.Equal(t, "dev", got.Name)
	assert.Equal(t, []string{"env:dev"}, got.Labels)
	assert.Equal(t, "https://dev.sprites.app", got.URL)
}

func TestInfoWithoutURL(t *testing.T) {
	api := newSpritesAPI(t)
	delete(api.addSprite("dev").info, "url")

	e := newEnv(t, newInfo(), "dev")
	require.NoError(t, runInfo(e.ctx, api.session()))
	assert.Contains(t, e.out.String(), "No URL available for sprite dev")
}

func TestInfoUnknownSprite(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newInfo(), "ghost")
	require.ErrorContains(t, runInfo(e.ctx, api.session()), "failed to get sprite info")
}

func TestConfigShow(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	// Defaults when the API reports no settings.
	e := newEnv(t, newConfig(), "dev")
	require.NoError(t, runConfigShow(e.ctx, api.session()))
	assert.Equal(t, "url-auth:       sprite\nprivate-access: admins\nlabels:         (none)\n", e.out.String())

	api.sprites["dev"].info["url_settings"] = map[string]any{"auth": "sprite", "private_access": "org_users"}
	api.sprites["dev"].info["labels"] = []string{"a", "b"}

	e = newEnv(t, newConfig(), "dev")
	require.NoError(t, runConfigShow(e.ctx, api.session()))
	assert.Equal(t, "url-auth:       sprite\nprivate-access: org_users\nlabels:         a, b\n", e.out.String())
}

func TestURL(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newURL(), "dev")
	require.NoError(t, runURL(e.ctx, api.session()))
	assert.Equal(t, "URL: https://dev.sprites.app\nAuth: sprite\n", e.out.String())

	delete(api.sprites["dev"].info, "url")

	e = newEnv(t, newURL(), "dev")
	require.NoError(t, runURL(e.ctx, api.session()))
	assert.Equal(t, "No URL available for sprite dev\n", e.out.String())
}

func TestURLUpdate(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	// "default" is the old name for "sprite".
	e := newEnv(t, newURLUpdate(), "--sprite", "dev", "--auth", "default")
	require.NoError(t, runURLUpdate(e.ctx, api.session()))

	assert.Equal(t, map[string]any{"auth": "sprite"}, api.sprites["dev"].info["url_settings"])
	assert.Contains(t, e.out.String(), "Auth: sprite")
	assert.Contains(t, e.out.String(), "URL: https://dev.sprites.app")

	for args, want := range map[string]string{"": "--auth is required", "nope": `invalid --auth "nope"`} {
		flags := []string{"--sprite", "dev"}
		if args != "" {
			flags = append(flags, "--auth", args)
		}

		e := newEnv(t, newURLUpdate(), flags...)
		require.ErrorContains(t, runURLUpdate(e.ctx, api.session()), want)
	}
}

func TestListInTerminal(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("alpha").info["url_settings"] = map[string]any{"auth": "public"}

	beta := api.addSprite("beta")
	beta.info["status"] = "cold"
	beta.info["last_running_at"] = time.Now().Add(-2 * time.Hour)

	e := newEnv(t, newList())
	e.ios.SetStdoutTTY(true)

	require.NoError(t, runList(e.ctx, api.session()))

	out := e.out.String()
	assert.Contains(t, out, "acme: 2 / 10 running, 0 / 10 warm")
	assert.Contains(t, out, "alpha.sprites.app (public)")
	assert.Contains(t, out, "2h ago")
	assert.Regexp(t, `(?i)name.*status.*url.*created.*last running`, out)
}

func TestListEmpty(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newList())
	e.ios.SetStdoutTTY(true)
	require.NoError(t, runList(e.ctx, api.session()))
	assert.Equal(t, "No sprites found in organization acme\n", e.out.String())

	e = newEnv(t, newList(), "--prefix", "zz")
	e.ios.SetStdoutTTY(true)
	require.NoError(t, runList(e.ctx, api.session()))
	assert.Equal(t, "No sprites found with prefix 'zz' in organization acme\n", e.out.String())
}

func TestListSummaryWithoutOrgTotals(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("a")
	api.addSprite("b").info["status"] = "warm"
	api.addSprite("c").info["status"] = "cold"

	result, err := api.session().Client.ListAllSpritesResult(context.Background(), "", nil)
	require.NoError(t, err)

	// Older API responses carry no totals, so they are counted here.
	result.Org = nil
	assert.Equal(t, "acme: 1 / 3 running, 1 / 3 warm (3 total)", listSummary(api.session(), result))
}

func TestListWatchStopsWhenCancelled(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("alpha")

	e := newEnv(t, newList(), "--watch")
	e.ios.SetStdoutTTY(true)

	ctx, cancel := context.WithCancel(e.ctx)

	done := make(chan error, 1)
	go func() { done <- runList(ctx, api.session()) }()

	// Wait for the first draw, then stop as Ctrl+C would.
	require.Eventually(t, func() bool { return len(api.requested()) > 0 }, 5*time.Second, 10*time.Millisecond)
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not stop when cancelled")
	}

	assert.Contains(t, e.out.String(), "alpha")
	assert.Contains(t, e.out.String(), "Press Ctrl+C to stop")
}

func TestListWatchPipedListsOnce(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("alpha")

	// Without a terminal to redraw, --watch has nothing to do.
	e := newEnv(t, newList(), "--watch")
	require.NoError(t, runList(e.ctx, api.session()))
	assert.Equal(t, "alpha\n", e.out.String())
}

func TestDestroyUnknownSprite(t *testing.T) {
	api := newSpritesAPI(t)

	e := newEnv(t, newDestroy(), "--force", "ghost")
	require.ErrorContains(t, runDestroy(e.ctx, api.session()), "failed to destroy sprite")
}

func TestDestroyKeepsParentDirectorySpriteFile(t *testing.T) {
	api := newSpritesAPI(t)
	api.addSprite("dev")

	e := newEnv(t, newDestroy(), "--yes")

	// The .sprite file is in a parent of the working directory. It names the
	// sprite to destroy, but it is not this directory's to remove.
	parent := e.workDir
	child := filepath.Join(parent, "sub")
	require.NoError(t, os.Mkdir(child, 0o755))
	require.NoError(t, writeSpriteFile(parent, "acme", "dev"))

	e.ctx = withWorkDir(e.ctx, child)

	require.NoError(t, runDestroy(e.ctx, api.session()))
	assert.NotContains(t, api.sprites, "dev")
	assert.FileExists(t, filepath.Join(parent, ".sprite"))
}
