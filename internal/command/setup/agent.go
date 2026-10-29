package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

const (
	embeddedEnv = "FLY_SETUP_EMBEDDED"

	hintRestart   = "Restart your coding agent(s) to load them."
	hintAuthorize = "The first MCP call opens a browser to authorize https://mcp.fly.io."
)

// loginFn and signupFn are the auth commands setup runs. Tests replace them.
var (
	loginFn  = auth.RunLogin
	signupFn = auth.RunSignup
)

func newAgent() *cobra.Command {
	const (
		short = "Install Fly skills and the hosted MCP server into local coding agents"
		long  = `Install the Fly agent skills and configure the hosted Fly MCP server
(https://mcp.fly.io) for every coding agent detected on this machine, then
make sure you are signed in to Fly.

Skills are installed the same way as 'fly skills install': into ~/.agents/skills
and the global skills directory of every detected agent (Claude Code, Codex,
Cursor, OpenCode, GitHub Copilot, Factory Droid, Pi and Antigravity). Use
--agent to pick agents explicitly. Files you have edited since the last install
are left alone; pass --force to overwrite them.

The MCP server is added under the name "fly" to each agent's global config.
No tokens, client ids or client secrets are written; the first call to the
server opens a browser to authorize https://mcp.fly.io. An entry that already
points at that URL is left as it is. An entry that points somewhere else is
left as it is unless --force is set. A config file that cannot be parsed is
left untouched and reported; the other agents are still configured.

Claude Code writes ~/.claude.json (mcpServers.fly). Cursor writes
~/.cursor/mcp.json (mcpServers.fly). OpenAI Codex writes ~/.codex/config.toml
(mcp_servers.fly). OpenCode writes ~/.config/opencode/opencode.json (mcp.fly,
or mcp.servers.fly when that object already exists). GitHub Copilot writes
~/.copilot/mcp-config.json (mcpServers.fly). Factory Droid writes
~/.factory/mcp.json (mcpServers.fly). Antigravity writes
~/.gemini/config/mcp_config.json (mcpServers.fly).

Pi gets skills only; it has no stable global MCP config. The universal
~/.agents directory gets skills only; it has no MCP config.

-y accepts the plan without prompting. A non-TTY stdin does the same and,
when you are not signed in, runs 'fly auth login'. --json prints the summary
as JSON and does not prompt.

The installer runs this command as FLY_SETUP_EMBEDDED=1 fly setup agent -y.
FLY_SETUP_EMBEDDED=1 suppresses the banner and the restart hint (the install
script prints its own) and keeps output to one line per agent.`
	)

	cmd := command.New("agent", short, long, runAgent)
	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.StringSlice{
			Name:        "agent",
			Description: skills.AgentFlagDescription,
		},
		flag.Bool{
			Name:        "force",
			Description: "Overwrite a fly MCP entry that points somewhere other than https://mcp.fly.io, and overwrite skills files you have modified locally",
		},
		flag.Bool{
			Name:        "skip-mcp",
			Description: "Do not configure the Fly MCP server",
		},
		flag.Bool{
			Name:        "skip-skills",
			Description: "Do not install Fly agent skills",
		},
		flag.Bool{
			Name:        "skip-login",
			Description: "Do not check whether you are signed in to Fly, and do not start a login",
		},
		flag.Yes(),
		flag.JSONOutput(),
	)

	return cmd
}

// agentResult is one agent's row in the text summary and the --json report.
type agentResult struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	SkillsDir   string `json:"skills_dir"`
	Skills      string `json:"skills"`
	SkillsError string `json:"skills_error,omitempty"`
	MCP         string `json:"mcp"`
	MCPPath     string `json:"mcp_path,omitempty"`
	MCPDetail   string `json:"mcp_detail,omitempty"`
}

func (r agentResult) succeeded() bool {
	switch {
	case r.Skills == "installed" || r.Skills == "updated" || r.Skills == "up to date" || r.Skills == "kept local changes":
		return true
	case strings.HasPrefix(r.Skills, "installed") || strings.HasPrefix(r.Skills, "updated"):
		return true
	default:
		return r.MCP == "configured" || r.MCP == "already configured"
	}
}

func (r agentResult) failed() bool {
	return r.Skills == "failed" || r.MCP == "failed"
}

func (r agentResult) line() string {
	var parts []string
	switch r.Skills {
	case "skipped":
		parts = append(parts, "skills skipped")
	case "failed":
		parts = append(parts, "skills failed ("+r.SkillsError+")")
	default:
		parts = append(parts, "skills "+r.Skills)
	}
	switch r.MCP {
	case "configured":
		parts = append(parts, "MCP configured → "+r.MCPPath)
	case "already configured":
		parts = append(parts, "MCP already configured → "+r.MCPPath)
	case "refused":
		parts = append(parts, "MCP left unchanged ("+r.MCPDetail+")")
	case "failed":
		parts = append(parts, "MCP failed ("+r.MCPDetail+")")
	case "skipped":
		parts = append(parts, "MCP skipped")
	case "unsupported":
		detail := r.MCPDetail
		if detail == "" {
			detail = "no MCP config for this agent"
		}
		parts = append(parts, "MCP skipped ("+detail+")")
	default:
		parts = append(parts, "MCP "+r.MCP)
	}

	return r.Name + ": " + strings.Join(parts, ", ")
}

type authReport struct {
	SignedIn bool   `json:"signed_in"`
	Email    string `json:"email,omitempty"`
	Action   string `json:"action"`
	Error    string `json:"error,omitempty"`
}

type setupReport struct {
	Embedded bool          `json:"embedded"`
	Auth     authReport    `json:"auth"`
	Agents   []agentResult `json:"agents"`
	Hints    []string      `json:"hints"`
}

func runAgent(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("could not determine home directory: %w", err)
	}

	slugs := flag.GetStringSlice(ctx, "agent")
	tools, err := skills.ResolveTools(home, slugs)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	embedded := os.Getenv(embeddedEnv) == "1"
	jsonOut := wantsJSON(ctx)
	// -y, a non-TTY stdin, and --json all skip the confirmation. --json has
	// to: a prompt would be written to the same stdout as the report.
	yes := flag.GetYes(ctx) || jsonOut || !io.IsStdinTTY()
	skipSkills := flag.GetBool(ctx, "skip-skills")
	skipMCP := flag.GetBool(ctx, "skip-mcp")
	force := flag.GetBool(ctx, "force")
	requested := len(slugs) > 0

	if !embedded && !jsonOut {
		fmt.Fprintf(io.Out, "\n%s\n\n", colorize.Bold("Fly Agent Setup"))
		fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Setting up:"))
		for _, tool := range tools {
			client := clientBySlug(tool.Slug)
			fmt.Fprintf(io.Out, "  %s\n", colorize.Bold(tool.Name))
			fmt.Fprintf(io.Out, "    skills  %s\n", colorize.Gray(tool.SkillsDir()))
			if len(client.rel) == 0 {
				fmt.Fprintf(io.Out, "    mcp     %s\n", colorize.Gray(client.note))
			} else {
				fmt.Fprintf(io.Out, "    mcp     %s\n", colorize.Gray(filepath.Join(append([]string{home}, client.rel...)...)))
			}
		}
		fmt.Fprintln(io.Out)
	}

	if !yes {
		confirmed, err := prompt.ConfirmYes(ctx, fmt.Sprintf("Set up %d agent%s?", len(tools), plural(len(tools))))
		if err != nil && !prompt.IsNonInteractive(err) {
			return err
		}
		if err == nil && !confirmed {
			return errors.New("setup cancelled")
		}
	}

	authResult := ensureAuth(ctx, yes)
	if !embedded && !jsonOut {
		printAuth(io, colorize, authResult)
	}

	results := make([]agentResult, len(tools))
	for i, tool := range tools {
		results[i] = agentResult{
			Slug:      tool.Slug,
			Name:      tool.Name,
			SkillsDir: tool.SkillsDir(),
		}
	}

	if skipSkills {
		for i := range results {
			results[i].Skills = "skipped"
		}
	} else {
		report, skillsErr := skills.SyncSkills(ctx, skills.BuildTargets(tools), "install", force, true)
		if report != nil {
			bySlug := map[string]skills.TargetSync{}
			for _, target := range report.Targets {
				bySlug[target.Slug] = target
			}
			for i, tool := range tools {
				if target, ok := bySlug[tool.Slug]; ok {
					results[i].Skills = target.Status
				}
			}
		}
		if skillsErr != nil {
			for i := range results {
				if results[i].Skills == "" {
					results[i].Skills = "failed"
					results[i].SkillsError = skillsErr.Error()
				}
			}
		}
	}

	if skipMCP {
		for i := range results {
			results[i].MCP = "skipped"
		}
	} else {
		for i, tool := range tools {
			outcome := installMCP(home, tool, force)
			results[i].MCP = outcome.Status
			results[i].MCPPath = outcome.Path
			results[i].MCPDetail = outcome.Detail
		}
	}

	report := setupReport{
		Embedded: embedded,
		Auth:     authResult,
		Agents:   results,
		Hints:    []string{hintRestart, hintAuthorize},
	}
	if err := printReport(ctx, report); err != nil {
		return err
	}

	if skipSkills && skipMCP {
		return nil
	}
	anySuccess := false
	requestedFailed := false
	for _, result := range results {
		if result.succeeded() {
			anySuccess = true
		}
		if requested && result.failed() {
			requestedFailed = true
		}
	}
	if requestedFailed {
		return errors.New("setup incomplete: a requested agent failed")
	}
	if !anySuccess {
		return errors.New("setup incomplete: nothing succeeded")
	}

	return nil
}

func wantsJSON(ctx context.Context) bool {
	if flag.GetBool(ctx, "json") {
		return true
	}

	return config.FromContext(ctx).JSONOutput
}

func printAuth(io *iostreams.IOStreams, colorize *iostreams.ColorScheme, authResult authReport) {
	switch {
	case authResult.Action == "skipped":
		fmt.Fprintf(io.Out, "Skipping Fly login.\n\n")
	case authResult.SignedIn && authResult.Email != "":
		fmt.Fprintf(io.Out, "Signed in as %s\n\n", colorize.Bold(authResult.Email))
	case authResult.SignedIn:
		fmt.Fprintf(io.Out, "Signed in to Fly.\n\n")
	case authResult.Error != "":
		fmt.Fprintf(io.Out, "%s Not signed in: %s\n", colorize.WarningIcon(), colorize.Yellow(authResult.Error))
		fmt.Fprintf(io.Out, "  Run `fly auth login` or `fly auth signup`. Setup will continue.\n\n")
	}
}

func printReport(ctx context.Context, report setupReport) error {
	io := iostreams.FromContext(ctx)
	if wantsJSON(ctx) {
		return render.JSON(io.Out, report)
	}

	colorize := io.ColorScheme()
	if !report.Embedded {
		fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Summary"))
	}
	for _, agent := range report.Agents {
		fmt.Fprintf(io.Out, "%s\n", agent.line())
	}
	if report.Auth.Error != "" && report.Embedded {
		fmt.Fprintf(io.Out, "Not signed in: %s\n", report.Auth.Error)
	}
	if !report.Embedded {
		fmt.Fprintln(io.Out)
		fmt.Fprintln(io.Out, hintRestart)
		fmt.Fprintln(io.Out, hintAuthorize)
	}

	return nil
}

func ensureAuth(ctx context.Context, yes bool) authReport {
	if flag.GetBool(ctx, "skip-login") {
		return authReport{Action: "skipped"}
	}
	if email, ok, err := currentEmail(ctx); ok {
		if err != nil {
			return authReport{SignedIn: true, Action: "failed", Error: err.Error()}
		}

		return authReport{SignedIn: true, Email: email, Action: "already signed in"}
	}
	if yes {
		return runAuth(ctx, "logged in", loginFn)
	}

	var index int
	err := prompt.Select(ctx, &index,
		"You are not signed in to Fly. Log in, or create an account?",
		"Log in (`fly auth login`)",
		"Log in (`fly auth login`)",
		"Sign up (`fly auth signup`)",
	)
	if err != nil {
		if prompt.IsNonInteractive(err) {
			return runAuth(ctx, "logged in", loginFn)
		}

		return authReport{Action: "failed", Error: err.Error()}
	}
	if index == 1 {
		return runAuth(ctx, "signed up", signupFn)
	}

	return runAuth(ctx, "logged in", loginFn)
}

func runAuth(ctx context.Context, action string, fn func(context.Context) error) authReport {
	if err := fn(ctx); err != nil {
		return authReport{Action: "failed", Error: err.Error()}
	}
	email := emailAfterLogin(ctx)

	return authReport{SignedIn: true, Email: email, Action: action}
}

func currentEmail(ctx context.Context) (email string, ok bool, err error) {
	client := flyutil.ClientFromContext(ctx)
	if client == nil || !client.Authenticated() {
		return "", false, nil
	}
	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return "", true, err
	}
	if user == nil {
		return "", true, nil
	}

	return user.Email, true, nil
}

// emailAfterLogin reads the token just saved. The client already in ctx was
// built before login, so it still looks signed out.
func emailAfterLogin(ctx context.Context) string {
	if email, ok, err := currentEmail(ctx); ok && err == nil && email != "" {
		return email
	}
	token, err := config.ReadAccessToken(state.ConfigFile(ctx))
	if err != nil || token == "" {
		return ""
	}
	user, err := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{AccessToken: token}).GetCurrentUser(ctx)
	if err != nil || user == nil {
		return ""
	}

	return user.Email
}

func plural(n int) string {
	if n == 1 {
		return ""
	}

	return "s"
}
