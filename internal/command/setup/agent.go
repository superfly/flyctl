package setup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/auth"
	"github.com/superfly/flyctl/internal/command/skills"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

const embeddedEnv = "FLY_SETUP_EMBEDDED"

const (
	restartHint   = "Restart your coding agent(s) to load them."
	authorizeHint = "The first MCP call opens a browser to authorize https://mcp.fly.io."
	authSkipHint  = "Not signed in. Run `fly auth login` or `fly auth signup` when you are ready."

	skillsInstalled = "installed"
)

const (
	optLogin  = "Log in (fly auth login)"
	optSignup = "Sign up (fly auth signup)"
	optSkip   = "Skip for now"
)

// confirmFn, loginFn, signupFn and selectFn are the prompt and auth entry
// points. Tests replace them so a unit test never opens a browser.
var (
	confirmFn = prompt.ConfirmYes
	loginFn   = auth.Login
	signupFn  = auth.Signup
	selectFn  = prompt.Select
)

var errNotSignedIn = errors.New("not signed in")

func newAgent() *cobra.Command {
	const (
		short = "Install Fly skills and the hosted MCP server into local coding agents"
		long  = `Detect the coding agents on this machine, install the Fly agent skills
into each of them, and add the hosted Fly MCP server at https://mcp.fly.io.

Skills are installed the same way as 'fly skills install': into ~/.agents/skills
and into the global skills directory of every detected coding agent (Claude
Code, Codex, Cursor, OpenCode, GitHub Copilot, Factory Droid, Pi and
Antigravity). Use --agent to pick agents explicitly. Files you have edited
are left alone. This command's --force does not overwrite those files; use
'fly skills install --force' for that.

The MCP server entry is named "fly" and stores only the URL. No access
tokens, client IDs or secrets are written. An entry that already points at
https://mcp.fly.io is left as it is. An entry that points somewhere else is
left as it is unless --force is passed. A config file that cannot be parsed
is left untouched and that agent is reported; the others are still set up.

Detected agents get that entry in their global config: Claude Code
(~/.claude.json), Cursor (~/.cursor/mcp.json), Codex (~/.codex/config.toml),
OpenCode (~/.config/opencode/opencode.json), GitHub Copilot
(~/.copilot/mcp-config.json), Factory Droid (~/.factory/mcp.json) and
Antigravity (~/.gemini/config/mcp_config.json). Pi and the universal
~/.agents target receive skills only. Pi has no stable global MCP config
that this command writes.

If you are not signed in, you are offered 'fly auth login' and
'fly auth signup'. -y runs login. A failed login does not stop the skills
or MCP setup; it is reported at the end. The first MCP call from an agent
opens a browser to authorize https://mcp.fly.io.

The installer script runs this command with the banner and the restart hint
suppressed, one line per agent:

  FLY_SETUP_EMBEDDED=1 fly setup agent -y

Pass --json to print the same summary as JSON.`
	)

	cmd := command.New("agent", short, long, runAgent)
	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.StringSlice{
			Name:        "agent",
			Description: skills.AgentFlagDescription(),
		},
		flag.Bool{
			Name:        "force",
			Description: "Overwrite an existing fly MCP entry that points somewhere other than https://mcp.fly.io",
		},
		flag.Bool{
			Name:        "skip-mcp",
			Description: "Do not write MCP configuration",
		},
		flag.Bool{
			Name:        "skip-skills",
			Description: "Do not install Fly agent skills",
		},
		flag.Bool{
			Name:        "skip-login",
			Description: "Do not prompt to log in or sign up",
		},
		flag.Yes(),
		flag.JSONOutput(),
	)

	return cmd
}

// report is the summary printed for humans and, with --json, emitted as JSON.
type report struct {
	OK            bool          `json:"ok"`
	Agents        []agentResult `json:"agents"`
	Auth          authResult    `json:"auth"`
	RestartHint   string        `json:"restart_hint,omitempty"`
	AuthorizeHint string        `json:"authorize_hint,omitempty"`
}

type agentResult struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	SkillsDir   string `json:"skills_dir"`
	Skills      string `json:"skills"`
	SkillsError string `json:"skills_error,omitempty"`
	MCP         string `json:"mcp"`
	MCPPath     string `json:"mcp_path,omitempty"`
	MCPDetail   string `json:"mcp_detail,omitempty"`
	MCPError    string `json:"mcp_error,omitempty"`
}

type authResult struct {
	OK      bool   `json:"ok"`
	Email   string `json:"email,omitempty"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

func runAgent(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	streams := iostreams.FromContext(ctx)
	slugs := flag.GetStringSlice(ctx, "agent")
	explicit := len(slugs) > 0
	assumeYes := flag.GetYes(ctx) || !streams.IsStdinTTY()
	asJSON := flag.GetBool(ctx, "json")
	embedded := embeddedMode()
	quiet := embedded || asJSON
	skipSkills := flag.GetBool(ctx, "skip-skills")
	skipMCP := flag.GetBool(ctx, "skip-mcp")

	tools, err := skills.ResolveTools(home, slugs)
	if err != nil {
		return err
	}
	if !quiet {
		printPlan(streams.Out, streams.ColorScheme(), home, tools, skipSkills, skipMCP)
	}
	if err := confirmSetup(ctx, len(tools), assumeYes); err != nil {
		return err
	}

	authReport := ensureAuth(ctx, streams, flag.GetBool(ctx, "skip-login"), assumeYes, quiet)
	agents := applySetup(ctx, home, tools, skipSkills, skipMCP, flag.GetBool(ctx, "force"), quiet)
	rep := makeReport(agents, authReport, embedded)
	exitErr := decideExit(agents, explicit)
	rep.OK = exitErr == nil

	if asJSON {
		enc := json.NewEncoder(streams.Out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(&rep); err != nil {
			return err
		}

		return exitErr
	}
	if embedded {
		printEmbedded(streams.Out, rep)
	} else {
		printHuman(streams.Out, streams.ColorScheme(), rep)
	}

	return exitErr
}

func embeddedMode() bool {
	_, ok := os.LookupEnv(embeddedEnv)

	return ok
}

func confirmSetup(ctx context.Context, n int, assumeYes bool) error {
	if assumeYes {
		return nil
	}
	confirmed, err := confirmFn(ctx, fmt.Sprintf("Set up %d agent(s)?", n))
	if err != nil && !prompt.IsNonInteractive(err) {
		return err
	}
	if err == nil && !confirmed {
		return errors.New("setup cancelled")
	}

	return nil
}

func ensureAuth(ctx context.Context, streams *iostreams.IOStreams, skip, assumeYes, quiet bool) authResult {
	if skip {
		return authResult{Skipped: true}
	}
	email, err := currentEmail(ctx)
	switch {
	case err == nil:
		return signedIn(streams, email, quiet)
	case !errors.Is(err, errNotSignedIn):
		return authResult{Error: oneLine(err.Error())}
	}

	action, authErr := chooseAuth(ctx, assumeYes)
	if authErr != nil {
		return authResult{Error: oneLine(authErr.Error())}
	}
	if action == optSkip {
		return authResult{Skipped: true}
	}

	fn := loginFn
	if action == optSignup {
		fn = signupFn
	}
	if err := callAuth(ctx, streams, fn); err != nil {
		return authResult{Error: oneLine(err.Error())}
	}

	email, err = currentEmail(ctx)
	switch {
	case errors.Is(err, errNotSignedIn):
		// Login reported success but the token is not visible yet. Do not
		// block skills or MCP on that.
		return authResult{OK: true}
	case err != nil:
		return authResult{OK: true, Error: oneLine(err.Error())}
	default:
		return signedIn(streams, email, quiet)
	}
}

func signedIn(streams *iostreams.IOStreams, email string, quiet bool) authResult {
	if !quiet && email != "" {
		fmt.Fprintf(streams.Out, "Signed in as %s\n", email)
	}

	return authResult{OK: true, Email: email}
}

func chooseAuth(ctx context.Context, assumeYes bool) (string, error) {
	if assumeYes {
		return optLogin, nil
	}
	var index int
	err := selectFn(ctx, &index, "Sign in to Fly", optLogin, optLogin, optSignup, optSkip)
	if prompt.IsNonInteractive(err) {
		return optLogin, nil
	}
	if err != nil {
		return "", err
	}
	switch index {
	case 1:
		return optSignup, nil
	case 2:
		return optSkip, nil
	default:
		return optLogin, nil
	}
}

// callAuth runs login or signup. --json moves that flow's stdout onto stderr
// so a "successfully logged in" line cannot break the JSON document.
func callAuth(ctx context.Context, streams *iostreams.IOStreams, fn func(context.Context) error) error {
	if !flag.GetBool(ctx, "json") || streams.ErrOut == nil {
		return fn(ctx)
	}
	saved := streams.Out
	streams.Out = streams.ErrOut
	defer func() { streams.Out = saved }()

	return fn(ctx)
}

// currentEmail reports the signed-in email. A client already on the context
// is used when it is authenticated. Otherwise the token file is read, because
// `fly auth login` saves the token without updating that client.
func currentEmail(ctx context.Context) (string, error) {
	if client := flyutil.ClientFromContext(ctx); client != nil && client.Authenticated() {
		return emailFrom(client.GetCurrentUser(ctx))
	}

	token, err := config.ReadAccessToken(state.ConfigFile(ctx))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", errNotSignedIn
		}

		return "", err
	}
	if token == "" {
		return "", errNotSignedIn
	}
	client := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{AccessToken: token})

	return emailFrom(client.GetCurrentUser(ctx))
}

func emailFrom(user *fly.User, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if user == nil || user.Email == "" {
		return "", errNotSignedIn
	}

	return user.Email, nil
}

func applySetup(ctx context.Context, home string, tools []skills.CodingTool, skipSkills, skipMCP, force, quiet bool) []agentResult {
	skillsStatus := skillsInstalled
	var skillsErr error
	if skipSkills {
		skillsStatus = statusSkipped
	} else if err := skills.InstallInto(ctx, skills.BuildTargets(tools), "install", false, quiet); err != nil {
		skillsStatus = statusFailed
		skillsErr = err
	}

	agents := make([]agentResult, 0, len(tools))
	for _, tool := range tools {
		result := agentResult{
			Slug:      tool.Slug,
			Name:      tool.Name,
			SkillsDir: tool.SkillsDir(),
			Skills:    skillsStatus,
		}
		if skillsErr != nil {
			result.SkillsError = oneLine(skillsErr.Error())
		}
		mcp := configureMCP(home, tool.Slug, force, skipMCP)
		result.MCP = mcp.Status
		result.MCPPath = mcp.Path
		result.MCPDetail = mcp.Detail
		if mcp.Err != nil {
			result.MCPError = oneLine(mcp.Err.Error())
		}
		agents = append(agents, result)
	}

	return agents
}

func makeReport(agents []agentResult, auth authResult, embedded bool) report {
	rep := report{Agents: agents, Auth: auth}
	if !embedded {
		rep.RestartHint = restartHint
		rep.AuthorizeHint = authorizeHint
	}

	return rep
}

func decideExit(agents []agentResult, explicit bool) error {
	var failed []string
	success := false
	for _, agent := range agents {
		if agent.Skills == skillsInstalled || agent.MCP == statusConfigured || agent.MCP == statusAlready {
			success = true
		}
		if explicit && (agent.Skills == statusFailed || agent.MCP == statusFailed) {
			failed = append(failed, agent.Name)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("setup failed for %s", strings.Join(failed, ", "))
	}
	if !success {
		return errors.New("setup did not install skills or configure MCP for any agent")
	}

	return nil
}

func printPlan(out io.Writer, colorize *iostreams.ColorScheme, home string, tools []skills.CodingTool, skipSkills, skipMCP bool) {
	fmt.Fprintf(out, "%s\n\n", colorize.Bold("Fly Agent Setup"))
	fmt.Fprintf(out, "This will set up %d agent(s):\n", len(tools))
	for _, tool := range tools {
		fmt.Fprintf(out, "\n  %s\n", colorize.Bold(tool.Name))
		fmt.Fprintf(out, "    skills  %s\n", skillsPlan(tool, skipSkills))
		fmt.Fprintf(out, "    MCP     %s\n", mcpPlan(home, tool.Slug, skipMCP))
	}
	fmt.Fprintln(out)
}

func skillsPlan(tool skills.CodingTool, skip bool) string {
	if skip {
		return "skipped (--skip-skills)"
	}

	return tool.SkillsDir()
}

func mcpPlan(home, slug string, skip bool) string {
	if skip {
		return "skipped (--skip-mcp)"
	}
	if client, ok := clientFor(slug); ok {
		return client.path(home)
	}

	return skillsOnlyDetail(slug)
}

func printHuman(out io.Writer, colorize *iostreams.ColorScheme, rep report) {
	fmt.Fprintln(out)
	for _, agent := range rep.Agents {
		fmt.Fprintf(out, "%s %s\n", summaryIcon(colorize, agent), agentLine(agent))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, rep.RestartHint)
	fmt.Fprintln(out, rep.AuthorizeHint)
	printAuthFooter(out, rep, false)
}

func printEmbedded(out io.Writer, rep report) {
	for _, agent := range rep.Agents {
		fmt.Fprintln(out, agentLine(agent))
	}
	printAuthFooter(out, rep, true)
}

func printAuthFooter(out io.Writer, rep report, embedded bool) {
	switch {
	case rep.Auth.Error != "":
		if embedded {
			fmt.Fprintf(out, "Sign-in failed: %s\n", rep.Auth.Error)

			return
		}
		fmt.Fprintf(out, "Sign-in failed: %s. Skills and MCP setup were not blocked.\n", rep.Auth.Error)
	case rep.Auth.Skipped && !embedded:
		fmt.Fprintln(out, authSkipHint)
	}
}

func summaryIcon(colorize *iostreams.ColorScheme, agent agentResult) string {
	switch {
	case agent.Skills == statusFailed || agent.MCP == statusFailed:
		return colorize.FailureIcon()
	case agent.MCP == statusConflict:
		return colorize.WarningIcon()
	default:
		return colorize.SuccessIcon()
	}
}

func agentLine(agent agentResult) string {
	skills := "skills " + agent.Skills
	if agent.Skills == statusFailed && agent.SkillsError != "" {
		skills = "skills failed (" + agent.SkillsError + ")"
	}
	mcp := agent.MCPDetail
	if mcp == "" {
		mcp = agent.MCP
	}

	return agent.Name + ": " + skills + ", MCP " + mcp
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
