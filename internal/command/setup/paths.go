package setup

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/superfly/flyctl/internal/command/skills"
)

// Config locations follow each client's own documentation.
//
//	CODEX_HOME          replaces ~/.codex, including config.toml
//	                    (https://developers.openai.com/codex/config-advanced)
//	CLAUDE_CONFIG_DIR   replaces ~/.claude
//	                    (https://code.claude.com/docs/en/env-vars).
//	                    ~/.claude.json moves with it to
//	                    $CLAUDE_CONFIG_DIR/.claude.json
//	                    (https://code.claude.com/docs/en/claude-directory:
//	                    "the same applies to .claude.json").
//	COPILOT_HOME        replaces ~/.copilot, including mcp-config.json
//	                    (https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)
//
// OpenCode resolves its global directory with xdg-basedir
// (packages/opencode/src/global/index.ts, and packages/core/src/global.ts on
// current dev): $XDG_CONFIG_HOME/opencode when XDG_CONFIG_HOME is set,
// otherwise ~/.config/opencode. That directory holds the MCP config and the
// skills directory. An existing opencode.jsonc is edited in place.
// Cursor, Factory Droid, Antigravity, and Pi have no confirmed override for
// the global MCP file this command writes.
func envPath(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return "", false
	}
	// Abs also cleans ".." so a relative override cannot escape by lexical
	// trickery. A leading ~ is not expanded.
	abs, err := filepath.Abs(value)
	if err != nil {
		return filepath.Clean(value), true
	}

	return abs, true
}

func claudeConfigDir(home string) string {
	if dir, ok := envPath("CLAUDE_CONFIG_DIR"); ok {
		return dir
	}

	return filepath.Join(home, ".claude")
}

func claudeMCPPath(home string) string {
	if dir, ok := envPath("CLAUDE_CONFIG_DIR"); ok {
		return filepath.Join(dir, ".claude.json")
	}

	return filepath.Join(home, ".claude.json")
}

func codexHome(home string) string {
	if dir, ok := envPath("CODEX_HOME"); ok {
		return dir
	}

	return filepath.Join(home, ".codex")
}

func copilotHome(home string) string {
	if dir, ok := envPath("COPILOT_HOME"); ok {
		return dir
	}

	return filepath.Join(home, ".copilot")
}

func opencodeConfigDir(home string) string {
	if dir, ok := envPath("XDG_CONFIG_HOME"); ok {
		return filepath.Join(dir, "opencode")
	}

	return filepath.Join(home, ".config", "opencode")
}

// opencodeMCPPath edits opencode.jsonc when that file is already present so a
// parallel opencode.json is not created beside it. A fresh install still
// writes opencode.json.
func opencodeMCPPath(home string) string {
	dir := opencodeConfigDir(home)
	jsonc := filepath.Join(dir, "opencode.jsonc")
	if isExistingFile(jsonc) {
		return jsonc
	}

	return filepath.Join(dir, "opencode.json")
}

func isExistingFile(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}

	return !info.IsDir()
}

func (c mcpClient) path(home string) string {
	switch c.slug {
	case "claude-code":
		return claudeMCPPath(home)
	case "codex":
		return filepath.Join(codexHome(home), "config.toml")
	case "copilot":
		return filepath.Join(copilotHome(home), "mcp-config.json")
	case "opencode":
		return opencodeMCPPath(home)
	default:
		parts := make([]string, 0, len(c.rel)+1)
		parts = append(parts, home)
		parts = append(parts, c.rel...)

		return filepath.Join(parts...)
	}
}

// prepareTools points Claude Code, Codex, Copilot, and OpenCode at the
// directories named by their config env vars, and auto-detects those tools
// when the override directory exists even if the default parent does not.
func prepareTools(home string, tools []skills.CodingTool, explicit bool) []skills.CodingTool {
	if !explicit {
		tools = appendEnvTools(home, tools)
	}
	tools = relocateTools(home, tools)
	sort.SliceStable(tools, func(i, j int) bool {
		return slugRank(tools[i].Slug) < slugRank(tools[j].Slug)
	})

	return tools
}

func appendEnvTools(home string, tools []skills.CodingTool) []skills.CodingTool {
	have := map[string]bool{}
	for _, tool := range tools {
		have[tool.Slug] = true
	}
	candidates := []struct {
		slug string
		dir  string
	}{
		{"claude-code", claudeConfigDir(home)},
		{"codex", codexHome(home)},
		{"copilot", copilotHome(home)},
		{"opencode", opencodeConfigDir(home)},
	}
	for _, candidate := range candidates {
		if have[candidate.slug] || !isDir(candidate.dir) {
			continue
		}
		// Only an override can name a directory other than the default parent.
		// The default parent is already handled by fly skills detection.
		if !envOverrides(candidate.slug) {
			continue
		}
		found, err := skills.ResolveTools(home, []string{candidate.slug})
		if err != nil || len(found) != 1 {
			continue
		}
		tools = append(tools, found[0])
		have[candidate.slug] = true
	}

	return tools
}

func envOverrides(slug string) bool {
	switch slug {
	case "claude-code":
		_, ok := envPath("CLAUDE_CONFIG_DIR")
		return ok
	case "codex":
		_, ok := envPath("CODEX_HOME")
		return ok
	case "copilot":
		_, ok := envPath("COPILOT_HOME")
		return ok
	case "opencode":
		_, ok := envPath("XDG_CONFIG_HOME")
		return ok
	default:
		return false
	}
}

func relocateTools(home string, tools []skills.CodingTool) []skills.CodingTool {
	out := make([]skills.CodingTool, len(tools))
	copy(out, tools)
	for i, tool := range out {
		switch tool.Slug {
		case "claude-code":
			out[i].Parent = claudeConfigDir(home)
		case "codex":
			out[i].Parent = codexHome(home)
		case "copilot":
			out[i].Parent = copilotHome(home)
		case "opencode":
			out[i].Parent = opencodeConfigDir(home)
		}
	}

	return out
}

func slugRank(slug string) int {
	for i, known := range skills.KnownAgentSlugs() {
		if known == slug {
			return i
		}
	}

	return len(skills.KnownAgentSlugs())
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
