package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command/skills"
)

// mcpURL is the hosted Fly MCP server. Clients only need the URL: the
// authorization server at https://fly.io/mcp/oauth publishes metadata,
// dynamic client registration and PKCE S256, and the browser consent does
// the rest. No token, client id or client secret is written to a config file.
const mcpURL = "https://mcp.fly.io"

const serverName = "fly"

// Shapes below were checked against current client docs and Railway's
// mcp/install.rs (the remote OAuth entries). Pi is skills-only: its global
// MCP file has not had a stable contract we are willing to write.
//
//   - Claude Code: ~/.claude.json, mcpServers.fly {type:http, url}. User scope,
//     https://code.claude.com/docs/en/mcp
//   - Cursor: ~/.cursor/mcp.json, mcpServers.fly {type:http, url}. The IDE
//     routes on url; the CLI accepts type "http". https://cursor.com/docs/mcp
//   - Codex: ~/.codex/config.toml, [mcp_servers.fly] url.
//     https://developers.openai.com/codex/config-reference
//   - OpenCode: ~/.config/opencode/opencode.json. Fresh files use the v1
//     mcp.<name> shape (type remote), which OpenCode v2 still reads. A file
//     that already has mcp.servers is updated there.
//     https://opencode.ai/docs/mcp-servers/ and https://opencode.ai/v2/docs/mcp-servers/
//   - GitHub Copilot CLI: ~/.copilot/mcp-config.json, type http, tools ["*"].
//     https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-mcp-servers
//   - Factory Droid: ~/.factory/mcp.json, type http.
//     https://docs.factory.ai/harness/mcp
//   - Antigravity: ~/.gemini/config/mcp_config.json, serverUrl.
//     https://antigravity.google/docs/mcp/

const (
	entryHTTP        = `{"type":"http","url":"https://mcp.fly.io"}`
	entryCursor      = entryHTTP
	entryCopilot     = `{"type":"http","url":"https://mcp.fly.io","tools":["*"]}`
	entryFactory     = `{"type":"http","url":"https://mcp.fly.io","disabled":false}`
	entryAntigravity = `{"serverUrl":"https://mcp.fly.io"}`
	entryOpenCode    = `{"type":"remote","url":"https://mcp.fly.io","enabled":true}`
)

type mcpClient struct {
	slug string
	// rel is the config path relative to the home directory.
	rel []string
	// container is the JSON object that holds server entries. nil for TOML
	// and for agents we do not write.
	container []string
	toml      bool
	entry     string
	fresh     string
	// note is set when this agent gets skills only.
	note string
}

func mcpClients() []mcpClient {
	return []mcpClient{
		{slug: "universal", note: "the universal .agents directory has no MCP config"},
		{
			slug:      "claude-code",
			rel:       []string{".claude.json"},
			container: []string{"mcpServers"},
			entry:     entryHTTP,
			fresh: `{
  "mcpServers": {
    "fly": {
      "type": "http",
      "url": "https://mcp.fly.io"
    }
  }
}
`,
		},
		{
			slug:  "codex",
			rel:   []string{".codex", "config.toml"},
			toml:  true,
			fresh: "[mcp_servers.fly]\nurl = \"https://mcp.fly.io\"\n",
		},
		{
			slug:      "cursor",
			rel:       []string{".cursor", "mcp.json"},
			container: []string{"mcpServers"},
			entry:     entryCursor,
			fresh: `{
  "mcpServers": {
    "fly": {
      "type": "http",
      "url": "https://mcp.fly.io"
    }
  }
}
`,
		},
		{
			slug:      "opencode",
			rel:       []string{".config", "opencode", "opencode.json"},
			container: []string{"mcp"},
			entry:     entryOpenCode,
			fresh: `{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "fly": {
      "type": "remote",
      "url": "https://mcp.fly.io",
      "enabled": true
    }
  }
}
`,
		},
		{
			slug:      "copilot",
			rel:       []string{".copilot", "mcp-config.json"},
			container: []string{"mcpServers"},
			entry:     entryCopilot,
			fresh: `{
  "mcpServers": {
    "fly": {
      "type": "http",
      "url": "https://mcp.fly.io",
      "tools": ["*"]
    }
  }
}
`,
		},
		{
			slug:      "factory-droid",
			rel:       []string{".factory", "mcp.json"},
			container: []string{"mcpServers"},
			entry:     entryFactory,
			fresh: `{
  "mcpServers": {
    "fly": {
      "type": "http",
      "url": "https://mcp.fly.io",
      "disabled": false
    }
  }
}
`,
		},
		{slug: "pi", note: "Pi has no stable global MCP config"},
		{
			slug:      "antigravity",
			rel:       []string{".gemini", "config", "mcp_config.json"},
			container: []string{"mcpServers"},
			entry:     entryAntigravity,
			fresh: `{
  "mcpServers": {
    "fly": {
      "serverUrl": "https://mcp.fly.io"
    }
  }
}
`,
		},
	}
}

func clientBySlug(slug string) mcpClient {
	for _, client := range mcpClients() {
		if client.slug == slug {
			return client
		}
	}

	return mcpClient{note: "no MCP config for this agent"}
}

// mcpOutcome is one agent's MCP result. Status is "configured",
// "already configured", "refused", "failed", "skipped" or "unsupported".
type mcpOutcome struct {
	Status string
	Path   string
	Detail string
}

func installMCP(home string, tool skills.CodingTool, force bool) mcpOutcome {
	client := clientBySlug(tool.Slug)
	if len(client.rel) == 0 {
		note := client.note
		if note == "" {
			note = "no MCP config for this agent"
		}

		return mcpOutcome{Status: "unsupported", Detail: note}
	}

	path := filepath.Join(append([]string{home}, client.rel...)...)
	data, err := os.ReadFile(path)
	missing := errors.Is(err, os.ErrNotExist)
	if err != nil && !missing {
		return mcpOutcome{Status: "failed", Path: path, Detail: err.Error()}
	}
	if missing {
		err = nil
	}

	var result mergeResult
	if client.toml {
		if missing {
			result = mergeResult{status: mergeWrote, data: []byte(client.fresh)}
		} else {
			result, err = mergeTOMLBytes(data, force)
		}
	} else if missing {
		result = mergeResult{status: mergeWrote, data: []byte(client.fresh)}
	} else {
		container := client.container
		if client.slug == "opencode" {
			container, err = opencodeContainer(data)
			if err != nil {
				return mcpOutcome{Status: "failed", Path: path, Detail: err.Error()}
			}
		}
		if err == nil {
			result, err = mergeJSONBytes(data, container, serverName, []byte(client.entry), force)
		}
	}
	if err != nil {
		return mcpOutcome{Status: "failed", Path: path, Detail: err.Error()}
	}

	switch result.status {
	case mergeAlready:
		return mcpOutcome{Status: "already configured", Path: path, Detail: result.detail}
	case mergeRefused:
		return mcpOutcome{Status: "refused", Path: path, Detail: result.detail}
	case mergeWrote:
		if err := writeAtomic(path, result.data); err != nil {
			return mcpOutcome{Status: "failed", Path: path, Detail: err.Error()}
		}

		return mcpOutcome{Status: "configured", Path: path}
	default:
		return mcpOutcome{Status: "failed", Path: path, Detail: "unknown merge result"}
	}
}

// opencodeContainer returns the object that holds server entries. OpenCode v2
// nests them under mcp.servers; v1 (and a fresh file) puts them directly
// under mcp. v2 still reads a v1 file, so a missing mcp.servers key stays v1.
func opencodeContainer(data []byte) ([]string, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return []string{"mcp"}, nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("could not parse JSON: %w", err)
	}
	mcp, ok := root["mcp"].(map[string]any)
	if !ok {
		return []string{"mcp"}, nil
	}
	servers, exists := mcp["servers"]
	if !exists {
		return []string{"mcp"}, nil
	}
	if _, ok := servers.(map[string]any); !ok {
		return nil, fmt.Errorf("mcp.servers is not a JSON object")
	}

	return []string{"mcp", "servers"}, nil
}

// writeAtomic creates or replaces path via a temp file in the same directory
// and keeps the mode of an existing file. helpers.WriteFileAtomically renames
// over the destination; Go's os.Rename replaces an existing file on Windows
// too (MOVEFILE_REPLACE_EXISTING).
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	info, err := os.Stat(path)
	switch {
	case err == nil:
		perm = info.Mode().Perm()
	case !errors.Is(err, os.ErrNotExist):
		return err
	}

	return helpers.WriteFileAtomically(path, data, perm)
}
