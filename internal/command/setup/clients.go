package setup

import (
	"encoding/json"
)

// hostedMCPURL is the plain hosted Fly MCP server. Clients that speak MCP
// OAuth discover https://fly.io/mcp/oauth from this URL; the browser consent
// handles authorization. Nothing here is a token, client ID or secret.
const hostedMCPURL = "https://mcp.fly.io"

// mcpServerName is the entry name written into every client.
const mcpServerName = "fly"

const (
	statusConfigured = "configured"
	statusAlready    = "already_configured"
	statusConflict   = "conflict"
	statusFailed     = "failed"
	statusSkipped    = "skipped"
)

// mcpFormat selects how a client's global config is merged.
type mcpFormat int

const (
	formatJSON mcpFormat = iota
	formatTOML
)

// mcpClient is one coding agent whose global MCP config we know how to merge.
// Paths and shapes were checked against each client's current docs and against
// railway setup agent's writers (railwayapp/cli src/commands/mcp/install.rs).
//
//	Claude Code     ~/.claude.json                         type http + url
//	Cursor          ~/.cursor/mcp.json                     url (Cursor infers HTTP)
//	Codex           ~/.codex/config.toml                   [mcp_servers.fly] url
//	OpenCode        ~/.config/opencode/opencode.json       mcp.fly, or mcp.servers.fly
//	                                                       when the file already uses mcp.servers
//	GitHub Copilot  ~/.copilot/mcp-config.json             type http + url + tools
//	Factory Droid   ~/.factory/mcp.json                    type http + url
//	Antigravity     ~/.gemini/config/mcp_config.json       serverUrl
//
// Pi's docs now describe ~/.pi/agent/mcp.json, but this command follows the
// settled decision to install skills only for Pi and say so.
type mcpClient struct {
	slug      string
	rel       []string
	format    mcpFormat
	container string
	entry     any
	// urlKeys are the fields that count as "where this entry points", in
	// preference order. Antigravity's documented key is serverUrl; url is
	// accepted so an already-correct url entry is not treated as a conflict.
	urlKeys []string
}

func (c mcpClient) entryJSON() ([]byte, error) {
	return json.Marshal(c.entry)
}

// mcpClients is the write set, in the same order as fly skills' coding tools.
var mcpClients = []mcpClient{
	{
		slug:      "claude-code",
		rel:       []string{".claude.json"},
		format:    formatJSON,
		container: "mcpServers",
		entry: httpEntry{
			Type: "http",
			URL:  hostedMCPURL,
		},
		urlKeys: []string{"url"},
	},
	{
		slug:      "cursor",
		rel:       []string{".cursor", "mcp.json"},
		format:    formatJSON,
		container: "mcpServers",
		entry:     cursorEntry{URL: hostedMCPURL},
		urlKeys:   []string{"url"},
	},
	{
		slug:   "codex",
		rel:    []string{".codex", "config.toml"},
		format: formatTOML,
	},
	{
		slug:      "opencode",
		rel:       []string{".config", "opencode", "opencode.json"},
		format:    formatJSON,
		container: "mcp",
		entry: opencodeEntry{
			Type:    "remote",
			URL:     hostedMCPURL,
			Enabled: true,
		},
		urlKeys: []string{"url"},
	},
	{
		slug:      "copilot",
		rel:       []string{".copilot", "mcp-config.json"},
		format:    formatJSON,
		container: "mcpServers",
		entry: copilotEntry{
			Type:  "http",
			URL:   hostedMCPURL,
			Tools: []string{"*"},
		},
		urlKeys: []string{"url"},
	},
	{
		slug:      "factory-droid",
		rel:       []string{".factory", "mcp.json"},
		format:    formatJSON,
		container: "mcpServers",
		entry: factoryEntry{
			Type:     "http",
			URL:      hostedMCPURL,
			Disabled: false,
		},
		urlKeys: []string{"url"},
	},
	{
		slug:      "antigravity",
		rel:       []string{".gemini", "config", "mcp_config.json"},
		format:    formatJSON,
		container: "mcpServers",
		entry:     antigravityEntry{ServerURL: hostedMCPURL},
		urlKeys:   []string{"serverUrl", "url"},
	},
}

type httpEntry struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// cursorEntry has no type. Cursor treats a url as a remote HTTP/SSE server.
type cursorEntry struct {
	URL string `json:"url"`
}

type opencodeEntry struct {
	Type    string `json:"type"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
}

type copilotEntry struct {
	Type  string   `json:"type"`
	URL   string   `json:"url"`
	Tools []string `json:"tools"`
}

type factoryEntry struct {
	Type     string `json:"type"`
	URL      string `json:"url"`
	Disabled bool   `json:"disabled"`
}

type antigravityEntry struct {
	ServerURL string `json:"serverUrl"`
}

func clientFor(slug string) (mcpClient, bool) {
	for _, client := range mcpClients {
		if client.slug == slug {
			return client, true
		}
	}

	return mcpClient{}, false
}

// skillsOnlyDetail is the MCP phrase for an agent we deliberately do not
// write a config for.
func skillsOnlyDetail(slug string) string {
	if slug == "pi" {
		return "skipped (no stable global MCP config)"
	}

	return "skipped (no MCP configuration for this agent)"
}
