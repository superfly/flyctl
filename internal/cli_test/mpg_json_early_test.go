package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
)

// mpgJSONBaseRef is the commit whose text output the MPG JSON work must not
// change. FLYCTL_MPGJSON_BASE_BIN may point at a prebuilt binary instead.
const mpgJSONBaseRef = "review/mpgjson-base"

// mpgJSONCommand is one way of spelling an MPG creation command.
type mpgJSONCommand struct {
	words   []string // command words, aliases included
	pos     []string // positional arguments
	flags   []string // flags the command needs to get going
	str     string   // a string flag of the command
	short   string   // that flag's shorthand
	maxArgs int      // the number of positional arguments the command accepts; -1: any
}

func mpgJSONCommands() []mpgJSONCommand {
	var cmds []mpgJSONCommand
	cmds = append(cmds,
		mpgJSONCommand{words: []string{"create"}, flags: []string{"--name", "my-db", "--region", "iad"}, str: "org", maxArgs: -1},
		mpgJSONCommand{words: []string{"fork"}, pos: []string{"src"}, str: "name", short: "n", maxArgs: 1},
		mpgJSONCommand{words: []string{"restore"}, pos: []string{"src"}, flags: []string{"--backup-id", "b1"}, str: "name", short: "n", maxArgs: 1},
	)
	for _, users := range []string{"users", "user"} {
		cmds = append(cmds, mpgJSONCommand{words: []string{users, "create"}, pos: []string{"cid"}, flags: []string{"--role", "writer"}, str: "username", short: "u", maxArgs: 1})
	}
	for _, dbs := range []string{"databases", "database", "db", "dbs"} {
		cmds = append(cmds, mpgJSONCommand{words: []string{dbs, "create"}, pos: []string{"cid"}, str: "name", short: "n", maxArgs: 1})
	}

	return cmds
}

// mpgJSONLayout is one way of laying out a command line. args builds it; the
// base binary runs baseArgs (or args) for the byte-for-byte text comparison.
type mpgJSONLayout struct {
	name     string
	env      []string
	json     bool // JSON mode
	compare  bool // text mode: compare with the base binary
	usage    bool // compare with the usage block masked, since it lists the new flag
	stdout   string
	stderr   string
	exitZero bool
	args     func(c mpgJSONCommand) []string
	baseArgs func(c mpgJSONCommand) []string
}

func (c mpgJSONCommand) line(extra ...string) []string {
	out := append([]string{"mpg"}, c.words...)
	out = append(out, c.pos...)
	out = append(out, c.flags...)
	return append(out, extra...)
}

// extra returns one more positional argument than the command accepts (or just
// one when it accepts any number), left empty for mpgJSONRows to name.
func (c mpgJSONCommand) extra() []string {
	if c.maxArgs < 0 {
		return []string{""}
	}
	return make([]string, c.maxArgs+1-len(c.pos))
}

func mpgJSONLayouts() []mpgJSONLayout {
	plain := func(c mpgJSONCommand) []string { return c.line() }
	return []mpgJSONLayout{
		{name: "verbose-between-words-j", json: true, args: func(c mpgJSONCommand) []string {
			return append([]string{"mpg", "--verbose"}, c.line("-j")[1:]...)
		}},
		{name: "json", json: true, args: func(c mpgJSONCommand) []string { return c.line("--json") }},
		{name: "json-true", json: true, args: func(c mpgJSONCommand) []string { return c.line("--json=true") }},
		{name: "short-group-json", json: true, args: func(c mpgJSONCommand) []string {
			if c.short == "" {
				return c.line("-j")
			}
			return c.line("-j"+c.short, "value")
		}},
		{name: "short-group-value", compare: true, args: func(c mpgJSONCommand) []string {
			if c.short == "" {
				return c.line("--" + c.str + "=j")
			}
			return c.line("-" + c.short + "j") // the flag's value is "j"
		}},
		{name: "value-swallows-json", compare: true, args: func(c mpgJSONCommand) []string {
			return c.line("--"+c.str, "--json")
		}},
		{name: "json-false", compare: true, args: func(c mpgJSONCommand) []string { return c.line("--json=false") }, baseArgs: plain},
		{name: "json-maybe", json: true, stderr: "invalid argument", args: func(c mpgJSONCommand) []string { return c.line("--json=maybe") }},
		{name: "env", env: []string{"FLY_JSON=1"}, json: true, args: plain},
		{name: "env-json-false", env: []string{"FLY_JSON=1"}, compare: true, args: func(c mpgJSONCommand) []string { return c.line("--json=false") }, baseArgs: plain},
		{name: "unknown-flag-json", json: true, stderr: "unknown flag", args: func(c mpgJSONCommand) []string { return c.line("--json", "--definitely-invalid") }},
		{name: "unknown-flag-short-group-json", json: true, stderr: "unknown flag", args: func(c mpgJSONCommand) []string {
			if c.short == "" {
				return c.line("--definitely-invalid", "-j")
			}
			return c.line("--definitely-invalid", "-j"+c.short, "value")
		}},
		// A flag error without JSON intent keeps the base routing.
		{name: "unknown-flag", compare: true, usage: true, stdout: "Usage:", args: func(c mpgJSONCommand) []string { return c.line("--definitely-invalid") }},
		{name: "unknown-flag-value-swallows-json", compare: true, usage: true, stdout: "Usage:", args: func(c mpgJSONCommand) []string {
			return c.line("--definitely-invalid", "--"+c.str, "--json")
		}},
		{name: "unknown-flag-env-json-false", env: []string{"FLY_JSON=1"}, compare: true, usage: true, stdout: "Usage:",
			args:     func(c mpgJSONCommand) []string { return c.line("--definitely-invalid", "--json=false") },
			baseArgs: func(c mpgJSONCommand) []string { return c.line("--definitely-invalid") }},
		{name: "unknown-flag-env", env: []string{"FLY_JSON=1"}, json: true, stderr: "unknown flag", args: func(c mpgJSONCommand) []string { return c.line("--definitely-invalid") }},
		{name: "extra-args-json", json: true, stderr: "Usage:", args: func(c mpgJSONCommand) []string {
			return c.line(append(c.extra(), "--json")...)
		}},
		{name: "extra-args", stdout: "Usage:", args: func(c mpgJSONCommand) []string {
			return c.line(c.extra()...)
		}},
		{name: "help", stdout: "Usage:", exitZero: true, args: func(c mpgJSONCommand) []string { return c.line("--help") }},
		{name: "help-json", json: true, stderr: "Usage:", exitZero: true, args: func(c mpgJSONCommand) []string { return c.line("--help", "--json") }},
		{name: "panic-json", json: true, env: []string{"FLYCTL_TEST_PANIC=injected"}, stderr: "injected", args: func(c mpgJSONCommand) []string { return c.line("--json") }},
		{name: "panic-text", env: []string{"FLYCTL_TEST_PANIC=injected"}, stdout: "injected", args: plain},
		{name: "text", compare: true, args: plain},
	}
}

// mpgJSONProfiles are the environments rows rotate through.
var mpgJSONProfiles = []struct {
	name    string
	env     []string
	config  string // config.yml; empty means none
	token   bool
	corrupt bool
}{
	{name: "plain"},
	{name: "gha-token", env: []string{"GITHUB_ACTIONS=true", "FLY_GHA_ERROR_ANNOTATION=true"}, token: true},
	{name: "debug-corrupt-config", env: []string{"LOG_LEVEL=debug"}, config: "a: [\n", corrupt: true},
	{name: "trace-gha-config", env: []string{"LOG_LEVEL=trace", "GITHUB_ACTIONS=true", "FLY_GHA_ERROR_ANNOTATION=true"}, config: "{}\n", token: true},
	{name: "debug-gha-corrupt-token", env: []string{"LOG_LEVEL=debug", "GITHUB_ACTIONS=true", "FLY_GHA_ERROR_ANNOTATION=true"}, config: "a: [\n", token: true, corrupt: true},
}

type mpgJSONRow struct {
	name       string
	args       []string
	baseArgs   []string
	env        []string
	config     string
	layout     mpgJSONLayout
	annotation bool
}

func mpgJSONRows(t *testing.T) []mpgJSONRow {
	perm, err := macaroon.New([]byte("fixture"), flyio.LocationPermission, macaroon.NewSigningKey())
	require.NoError(t, err)
	token, err := perm.String()
	require.NoError(t, err)

	var rows []mpgJSONRow
	add := func(name string, layout mpgJSONLayout, args, baseArgs []string, i int) {
		p := mpgJSONProfiles[i%len(mpgJSONProfiles)]
		env := append(append([]string{}, p.env...), layout.env...)
		if p.token {
			env = append(env, "FLY_ACCESS_TOKEN=FlyV1 "+token)
		}
		if baseArgs == nil {
			baseArgs = args
		}
		rows = append(rows, mpgJSONRow{
			name: name + "/" + p.name, args: args, baseArgs: baseArgs, env: env, config: p.config, layout: layout,
			annotation: strings.Contains(strings.Join(p.env, " "), "GITHUB_ACTIONS=true"),
		})
	}
	for ci, c := range mpgJSONCommands() {
		for li, l := range mpgJSONLayouts() {
			args := l.args(c)
			for i, a := range args {
				if a == "" {
					args[i] = fmt.Sprintf("extra%d", i)
				}
			}
			var baseArgs []string
			if l.baseArgs != nil {
				baseArgs = l.baseArgs(c)
			}
			if c.maxArgs < 0 && strings.HasPrefix(l.name, "extra-args") {
				// Not a usage error: the command runs as it does in the base.
				l.stdout, l.stderr, l.compare = "", "", !l.json
			}
			add(strings.Join(c.words, "-")+"/"+l.name, l, args, baseArgs, ci+li)
		}
	}
	// Commands without the annotation keep their routing, --json or not.
	unrelated := mpgJSONLayout{name: "unrelated", compare: true}
	for i, args := range [][]string{
		{"apps", "create", "my-app", "--org", "personal", "--json"},
		{"apps", "create", "--json", "--definitely-invalid"},
		{"mpg", "list", "--json"},
		{"mpg", "users", "list", "cid", "--json"},
		{"mpg", "nonexistent", "--json"},
	} {
		add("unrelated/"+strings.Join(args, "_"), unrelated, args, nil, i)
	}
	// Reviewer cases: flag errors without JSON intent, including with a
	// config load error (profiles 2 and 4 have a corrupt config).
	flagError := mpgJSONLayout{name: "flag-error", compare: true, usage: true, stdout: "Usage:"}
	flagErrorJSON := mpgJSONLayout{name: "flag-error-json", json: true, stderr: "invalid argument"}
	for i, args := range [][]string{
		{"mpg", "create", "--name", "x", "--region", "iad", "--definitely-invalid"},
		{"mpg", "fork", "src", "--not-a-flag"},
		{"mpg", "create", "--volume-size=nope"},
	} {
		for _, p := range []int{0, 2, 4} {
			add(fmt.Sprintf("flag-error/%d/%s", i, strings.Join(args, "_")), flagError, args, nil, p)
		}
	}
	add("flag-error-json/volume-size", flagErrorJSON, []string{"mpg", "create", "--volume-size=nope", "-j"}, nil, 2)

	return rows
}

// TestMPGJSONStdout checks, for every MPG creation command and alias against
// every argument layout, environment and early exit, that JSON mode leaves
// stdout empty or holding exactly one JSON document, and that text mode
// prints exactly what the base binary prints.
func TestMPGJSONStdout(t *testing.T) {
	if testing.Short() {
		t.Skip("builds flyctl twice")
	}
	dir := t.TempDir()
	bin := buildFlyctl(t, mpgJSONModuleRoot(t), filepath.Join(dir, "new", "fly"))
	base := os.Getenv("FLYCTL_MPGJSON_BASE_BIN")
	if base == "" {
		base = buildMPGJSONBase(t, filepath.Join(dir, "base"))
	}

	t.Run("update-check-tty", func(t *testing.T) {
		t.Parallel()
		testMPGJSONUpdateCheckTTY(t, bin)
	})
	for _, row := range mpgJSONRows(t) {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			work := t.TempDir()
			stdout, stderr, code := runMPGJSONBinary(t, bin, work, row.config, row.env, row.args)
			msg := fmt.Sprintf("args=%q env=%q exit=%d\nstdout=%q\nstderr=%q", row.args, row.env, code, stdout, stderr)
			l := row.layout

			if l.json {
				if trimmed := bytes.TrimSpace(stdout); len(trimmed) > 0 {
					require.True(t, json.Valid(trimmed), "stdout is not exactly one JSON document\n%s", msg)
				} else if !l.exitZero {
					require.NotZero(t, code, "empty stdout needs a failing exit\n%s", msg)
				}
				if l.exitZero {
					require.Zero(t, code, msg)
				}
				if l.stderr != "" {
					require.Contains(t, string(stderr), l.stderr, msg)
				}
				if row.annotation && code == 1 {
					require.Contains(t, string(stderr), "::error title=flyctl error::", msg)
				}

				return
			}

			if l.stdout != "" {
				require.Contains(t, string(stdout), l.stdout, msg)
			}
			if l.exitZero {
				require.Zero(t, code, msg)
			}
			if !l.compare {
				return
			}
			if base == "" {
				t.Skipf("no base binary for %s", mpgJSONBaseRef)
			}
			baseOut, baseErr, baseCode := runMPGJSONBinary(t, base, work, row.config, row.env, row.baseArgs)
			require.Equal(t, baseCode, code, msg)
			if l.usage {
				baseOut, stdout = maskMPGJSONUsage(baseOut), maskMPGJSONUsage(stdout)
			}
			require.Equal(t, normalizeMPGJSONOutput(baseOut), normalizeMPGJSONOutput(stdout), "stdout differs from %s\n%s", mpgJSONBaseRef, msg)
			require.Equal(t, normalizeMPGJSONOutput(baseErr), normalizeMPGJSONOutput(stderr), "stderr differs from %s\n%s", mpgJSONBaseRef, msg)
		})
	}
}

// testMPGJSONUpdateCheckTTY runs a JSON-mode command with stderr on a terminal
// and stdout piped: like the base, which looks at the real stdout, the update
// check must see a pipe and skip querying for a new release, and the client
// signals must report a non-interactive run.
func testMPGJSONUpdateCheckTTY(t *testing.T, bin string) {
	master, slave := openPTY(t)
	work := t.TempDir()
	home := filepath.Join(work, "home")
	configDir := filepath.Join(home, ".fly")
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	const closed = "http://127.0.0.1:1"
	cmd := exec.CommandContext(t.Context(), bin, "mpg", "create", "--name", "x", "--region", "iad", "--json")
	cmd.Dir = work
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "FLY_CONFIG_DIR=" + configDir, "TMPDIR=" + work,
		"FLY_API_BASE_URL=" + closed, "FLY_FLAPS_BASE_URL=" + closed, "FLY_METRICS_BASE_URL=" + closed,
		"FLY_TRACE_COLLECTOR_URL=127.0.0.1:1", "HTTP_PROXY=" + closed, "HTTPS_PROXY=" + closed,
		"FLY_SEND_METRICS=0", "FLY_NO_INCIDENTS_CHECK=1", "NO_COLOR=1", "LOG_LEVEL=debug",
	}
	var stdout bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, slave
	output := make(chan []byte, 1)
	go func() {
		var data []byte
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			data = append(data, buf[:n]...)
			if err != nil {
				break
			}
		}
		output <- data
	}()
	runErr := cmd.Run()
	require.NoError(t, slave.Close())
	var stderr []byte
	select {
	case stderr = <-output:
	case <-time.After(10 * time.Second):
		_ = master.Close()
		stderr = <-output
	}
	require.Error(t, runErr, "no network, so the command fails")
	require.Empty(t, stdout.String(), "stderr=%q", stderr)
	require.Contains(t, string(stderr), "skipped querying for new release")
	// Likewise the client signals, sent with every API request: stdout is a pipe.
	require.Contains(t, string(stderr), "client signals: enabled interactive=false")
}

// runMPGJSONBinary runs bin with HOME and the config directory under work,
// reset for every run so that both binaries see the same paths and files, and
// with every network destination pointed at a closed port.
func runMPGJSONBinary(t *testing.T, bin, work, config string, env, args []string) (stdout, stderr []byte, code int) {
	t.Helper()
	home := filepath.Join(work, "home")
	configDir := filepath.Join(home, ".fly")
	require.NoError(t, os.RemoveAll(home))
	require.NoError(t, os.MkdirAll(configDir, 0o700))
	if config != "" {
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "config.yml"), []byte(config), 0o600))
	}
	const closed = "http://127.0.0.1:1"
	cmd := exec.CommandContext(t.Context(), bin, args...)
	cmd.Dir = work
	cmd.Env = append([]string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "FLY_CONFIG_DIR=" + configDir, "TMPDIR=" + work,
		"FLY_API_BASE_URL=" + closed, "FLY_FLAPS_BASE_URL=" + closed, "FLY_METRICS_BASE_URL=" + closed,
		"FLY_SYNTHETICS_BASE_URL=" + closed, "FLY_TRACE_COLLECTOR_URL=127.0.0.1:1",
		"HTTP_PROXY=" + closed, "HTTPS_PROXY=" + closed, "http_proxy=" + closed, "https_proxy=" + closed,
		"NO_PROXY=", "no_proxy=",
		"FLY_NO_UPDATE_CHECK=1", "FLY_SEND_METRICS=0", "NO_COLOR=1",
	}, env...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		code = exit.ExitCode()
	default:
		require.NoError(t, err)
	}

	return out.Bytes(), errOut.Bytes(), code
}

var mpgJSONVolatile = []struct {
	re   *regexp.Regexp
	with string
}{
	// trace spans and debug logs carry times, durations and random ids
	{regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`), "<time>"},
	{regexp.MustCompile(`\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(\.\d+)?`), "<time>"},
	{regexp.MustCompile(`\b[0-9a-f]{16}([0-9a-f]{16})?\b`), "<id>"},
	{regexp.MustCompile(`\b\d+(\.\d+)?(ns|µs|ms|s|m|h)\b`), "<duration>"},
	// crash reports and stack traces carry goroutine ids and addresses
	{regexp.MustCompile(`0x[0-9a-f]+`), "<addr>"},
	{regexp.MustCompile(`goroutine \d+`), "goroutine <n>"},
	{regexp.MustCompile(`"(StartTime|EndTime|Time)": [^\n]*`), `"$1": <time>`},
	{regexp.MustCompile(`"(DroppedAttributes|DroppedEvents|DroppedLinks|ChildSpanCount)": \d+`), `"$1": <n>`},
}

// mpgJSONUsage matches the usage block Cobra prints after a flag error. It
// lists --json and the examples, which the base does not have.
var mpgJSONUsage = regexp.MustCompile(`(?ms)^Usage:\n.*?^Global Flags:\n(?:[ \t]+.*\n)*`)

func maskMPGJSONUsage(b []byte) []byte {
	return mpgJSONUsage.ReplaceAll(b, []byte("<usage>\n"))
}

func normalizeMPGJSONOutput(b []byte) string {
	s := string(b)
	for _, v := range mpgJSONVolatile {
		s = v.re.ReplaceAllString(s, v.with)
	}
	return s
}

func mpgJSONModuleRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	require.NoError(t, err)
	return filepath.Dir(strings.TrimSpace(string(out)))
}

// buildFlyctl builds a production flyctl with fixed build information, so the
// panic handler runs and the version string is the same for both binaries.
func buildFlyctl(t *testing.T, src, out string) string {
	t.Helper()
	const pkg = "github.com/superfly/flyctl/internal/buildinfo"
	ldflags := fmt.Sprintf("-X '%s.buildDate=2000-01-01T00:00:00Z' -X '%s.buildVersion=9.9.9' -X '%s.commit=test'", pkg, pkg, pkg)
	cmd := exec.CommandContext(t.Context(), "go", "build", "-buildvcs=false", "-tags", "production", "-ldflags", ldflags, "-o", out, ".")
	cmd.Dir = src
	start := time.Now()
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "go build in %s: %s", src, output)
	t.Logf("built %s in %s", out, time.Since(start).Round(time.Second))

	return out
}

// buildMPGJSONBase builds mpgJSONBaseRef, or returns "" when the repository
// does not have it.
func buildMPGJSONBase(t *testing.T, dir string) string {
	t.Helper()
	root := mpgJSONModuleRoot(t)
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", mpgJSONBaseRef+"^{commit}").Run(); err != nil {
		t.Logf("%s is not available; text rows are not compared", mpgJSONBaseRef)
		return ""
	}
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	archive := exec.Command("git", "-C", root, "archive", mpgJSONBaseRef)
	extract := exec.Command("tar", "-x", "-C", src)
	pipe, err := archive.StdoutPipe()
	require.NoError(t, err)
	extract.Stdin = pipe
	require.NoError(t, extract.Start())
	require.NoError(t, archive.Run())
	require.NoError(t, extract.Wait())

	return buildFlyctl(t, src, filepath.Join(dir, "fly"))
}
