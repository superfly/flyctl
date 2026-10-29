package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/command/skills"
)

func mustTools(home string) []skills.CodingTool {
	tools, err := skills.ResolveTools(home, []string{
		"universal", "claude-code", "codex", "cursor", "opencode",
		"copilot", "factory-droid", "pi", "antigravity",
	})
	if err != nil {
		panic(err)
	}

	return tools
}

func configPath(home string, client mcpClient) string {
	return filepath.Join(append([]string{home}, client.rel...)...)
}

func TestEachMCPWriter(t *testing.T) {
	for _, client := range mcpClients() {
		if len(client.rel) == 0 {
			continue
		}
		t.Run(client.slug, func(t *testing.T) {
			if client.toml {
				testCodexWriter(t, client)
				return
			}
			testJSONWriter(t, client)
		})
	}
}

func testJSONWriter(t *testing.T, client mcpClient) {
	t.Helper()
	tool := mustTool(client.slug)

	t.Run("fresh", func(t *testing.T) {
		home := t.TempDir()
		outcome := installMCP(home, tool, false)
		require.Equal(t, "configured", outcome.Status)
		got := readFile(t, configPath(home, client))
		assertFlyPointsAt(t, client, got, mcpURL)
		assertNoSecrets(t, got)
		if runtime.GOOS != "windows" {
			info, err := os.Stat(configPath(home, client))
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
		}
	})

	t.Run("preserves other servers", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := otherServersJSON(client)
		writeMode(t, path, original, 0o640)

		outcome := installMCP(home, tool, false)
		require.Equal(t, "configured", outcome.Status, outcome.Detail)
		got := readFile(t, path)
		assertPreservedAround(t, original, got, preservedSnippet(client))
		assertFlyPointsAt(t, client, got, mcpURL)
		assert.Contains(t, string(got), `"keep": true`)
		if runtime.GOOS != "windows" {
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
		}
	})

	t.Run("already configured", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := alreadyJSON(client)
		writeMode(t, path, original, 0o644)
		outcome := installMCP(home, tool, false)
		require.Equal(t, "already configured", outcome.Status, outcome.Detail)
		assert.Equal(t, original, readFile(t, path))
	})

	t.Run("refuses a different entry", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := differentJSON(client)
		writeMode(t, path, original, 0o644)
		outcome := installMCP(home, tool, false)
		require.Equal(t, "refused", outcome.Status, outcome.Detail)
		assert.Contains(t, outcome.Detail, "https://example.invalid")
		assert.Contains(t, outcome.Detail, "--force")
		assert.Equal(t, original, readFile(t, path))
	})

	t.Run("force replaces a different entry", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := differentJSON(client)
		writeMode(t, path, original, 0o644)
		outcome := installMCP(home, tool, true)
		require.Equal(t, "configured", outcome.Status, outcome.Detail)
		got := readFile(t, path)
		assertFlyPointsAt(t, client, got, mcpURL)
		assert.Contains(t, string(got), `"keep": true`)
		assert.NotContains(t, string(got), "https://example.invalid")
	})

	t.Run("unparseable file is left untouched", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := []byte("{not json, and a brace } in the mess\n")
		writeMode(t, path, original, 0o600)
		outcome := installMCP(home, tool, true)
		require.Equal(t, "failed", outcome.Status)
		assert.Contains(t, outcome.Detail, "could not parse JSON")
		assert.Equal(t, original, readFile(t, path))
	})
}

func testCodexWriter(t *testing.T, client mcpClient) {
	t.Helper()
	tool := mustTool(client.slug)

	t.Run("fresh", func(t *testing.T) {
		home := t.TempDir()
		outcome := installMCP(home, tool, false)
		require.Equal(t, "configured", outcome.Status, outcome.Detail)
		got := readFile(t, configPath(home, client))
		assert.Contains(t, string(got), "[mcp_servers.fly]")
		assert.Contains(t, string(got), `url = "https://mcp.fly.io"`)
		assertNoSecrets(t, got)
	})

	t.Run("preserves comments and other tables", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := []byte("# keep me\nmodel = \"gpt-5\"\n\n[mcp_servers.other]\ncommand = \"echo\" # tool\n\n[profiles.dev]\n# still here\nsandbox_mode = \"workspace-write\"\n")
		writeMode(t, path, original, 0o640)
		outcome := installMCP(home, tool, false)
		require.Equal(t, "configured", outcome.Status, outcome.Detail)
		got := readFile(t, path)
		assert.True(t, bytes.HasPrefix(got, original), "original bytes should be an untouched prefix")
		assert.Contains(t, string(got), `url = "https://mcp.fly.io"`)
		assert.Contains(t, string(got), "# keep me")
		assert.Contains(t, string(got), "# still here")
		assert.Contains(t, string(got), "command = \"echo\" # tool")
		assertNoSecrets(t, got)
		if runtime.GOOS != "windows" {
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(0o640), info.Mode().Perm())
		}
	})

	t.Run("already configured", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := []byte("# keep me\n[mcp_servers.fly]\n# mine\nurl = \"https://mcp.fly.io\"\nenabled = true\n\n[profiles.dev]\nx = 1\n")
		writeMode(t, path, original, 0o644)
		outcome := installMCP(home, tool, false)
		require.Equal(t, "already configured", outcome.Status, outcome.Detail)
		assert.Equal(t, original, readFile(t, path))
	})

	t.Run("refuses a different entry", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := []byte("# keep me\n[mcp_servers.fly]\nurl = \"https://example.invalid\"\ncommand = \"nope\"\n\n[profiles.dev]\n# still here\nsandbox_mode = \"workspace-write\"\n")
		writeMode(t, path, original, 0o644)
		outcome := installMCP(home, tool, false)
		require.Equal(t, "refused", outcome.Status, outcome.Detail)
		assert.Equal(t, original, readFile(t, path))
	})

	t.Run("force replaces a different entry", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := []byte("# keep me\n[mcp_servers.fly]\nurl = \"https://example.invalid\"\ncommand = \"nope\"\n\n[profiles.dev]\n# still here\nsandbox_mode = \"workspace-write\"\n")
		writeMode(t, path, original, 0o644)
		outcome := installMCP(home, tool, true)
		require.Equal(t, "configured", outcome.Status, outcome.Detail)
		got := string(readFile(t, path))
		assert.Contains(t, got, "# keep me")
		assert.Contains(t, got, "# still here")
		assert.Contains(t, got, `url = "https://mcp.fly.io"`)
		assert.Contains(t, got, `sandbox_mode = "workspace-write"`)
		assert.NotContains(t, got, "example.invalid")
		assert.NotContains(t, got, "nope")
	})

	t.Run("unparseable file is left untouched", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := []byte("[mcp_servers\nthis is = not toml\n")
		writeMode(t, path, original, 0o644)
		outcome := installMCP(home, tool, true)
		require.Equal(t, "failed", outcome.Status)
		assert.Contains(t, outcome.Detail, "could not parse TOML")
		assert.Equal(t, original, readFile(t, path))
	})

	t.Run("dotted url is replaced in place", func(t *testing.T) {
		home := t.TempDir()
		path := configPath(home, client)
		original := []byte("# keep me\nmcp_servers.fly.url = \"https://example.invalid\"\nmodel = \"gpt-5\"\n")
		writeMode(t, path, original, 0o644)
		refused := installMCP(home, tool, false)
		require.Equal(t, "refused", refused.Status, refused.Detail)
		assert.Equal(t, original, readFile(t, path))

		outcome := installMCP(home, tool, true)
		require.Equal(t, "configured", outcome.Status, outcome.Detail)
		got := string(readFile(t, path))
		assert.Contains(t, got, "# keep me")
		assert.Contains(t, got, `model = "gpt-5"`)
		assert.Contains(t, got, `mcp_servers.fly.url = "https://mcp.fly.io"`)
		assert.NotContains(t, got, "example.invalid")
	})
}

func TestOpenCodeV2ServersArePreserved(t *testing.T) {
	home := t.TempDir()
	client := clientBySlug("opencode")
	path := configPath(home, client)
	original := []byte("{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"mcp\": {\n    \"servers\": {\n      \"other\": {\"type\": \"remote\", \"url\": \"https://example.com\", \"note\": \"braces } { stay\"}\n    }\n  },\n  \"keep\": true\n}\n")
	writeMode(t, path, original, 0o644)

	outcome := installMCP(home, mustTool("opencode"), false)
	require.Equal(t, "configured", outcome.Status, outcome.Detail)
	got := readFile(t, path)
	assertPreservedAround(t, original, got, `"note": "braces } { stay"`)
	assert.Contains(t, string(got), `"keep": true`)
	assert.Contains(t, string(got), `"$schema"`)

	var root map[string]any
	require.NoError(t, json.Unmarshal(got, &root))
	servers := root["mcp"].(map[string]any)["servers"].(map[string]any)
	fly := servers["fly"].(map[string]any)
	assert.Equal(t, mcpURL, fly["url"])
	assert.Equal(t, "remote", fly["type"])
	other := servers["other"].(map[string]any)
	assert.Equal(t, "https://example.com", other["url"])
}

func TestAntigravityURLCountsAsConfigured(t *testing.T) {
	home := t.TempDir()
	client := clientBySlug("antigravity")
	path := configPath(home, client)
	original := []byte("{\n  \"mcpServers\": {\n    \"fly\": {\"url\": \"https://mcp.fly.io\"},\n    \"other\": {\"serverUrl\": \"https://example.com\"}\n  }\n}\n")
	writeMode(t, path, original, 0o644)
	outcome := installMCP(home, mustTool("antigravity"), false)
	require.Equal(t, "already configured", outcome.Status, outcome.Detail)
	assert.Equal(t, original, readFile(t, path))
}

func TestPiAndUniversalAreSkillsOnly(t *testing.T) {
	home := t.TempDir()
	for _, slug := range []string{"pi", "universal"} {
		outcome := installMCP(home, mustTool(slug), true)
		assert.Equal(t, "unsupported", outcome.Status, slug)
		assert.NotEmpty(t, outcome.Detail, slug)
	}
	assert.NoFileExists(t, filepath.Join(home, ".pi", "agent", "mcp.json"))
	assert.NoFileExists(t, filepath.Join(home, ".agents", "mcp.json"))
}

func TestFreshFilesContainNoSecrets(t *testing.T) {
	for _, client := range mcpClients() {
		if client.fresh == "" && client.entry == "" {
			continue
		}
		assertNoSecrets(t, []byte(client.fresh+"\n"+client.entry))
	}
}

func mustTool(slug string) skills.CodingTool {
	for _, tool := range mustTools("/") {
		if tool.Slug == slug {
			tool.Parent = ""
			return tool
		}
	}
	panic("unknown slug " + slug)
}

func otherServersJSON(client mcpClient) []byte {
	if client.slug == "opencode" {
		return []byte("{\n  \"mcp\": {\n    \"other\": {\"type\": \"remote\", \"url\": \"https://example.com\", \"note\": \"braces } { stay\"}\n  },\n  \"keep\": true\n}\n")
	}

	return []byte("{\n  \"mcpServers\": {\n    \"other\": {\"url\": \"https://example.com\", \"note\": \"braces } { stay\"}\n  },\n  \"keep\": true\n}\n")
}

func preservedSnippet(client mcpClient) string {
	return `"note": "braces } { stay"`
}

func alreadyJSON(client mcpClient) []byte {
	if client.slug == "opencode" {
		return []byte("{\n  \"mcp\": {\n    \"other\": {\"type\": \"remote\", \"url\": \"https://example.com\"},\n    \"fly\": {\"url\": \"https://mcp.fly.io\", \"type\": \"remote\", \"enabled\": true}\n  },\n  \"keep\": true\n}\n")
	}
	if client.slug == "antigravity" {
		return []byte("{\n  \"mcpServers\": {\n    \"other\": {\"serverUrl\": \"https://example.com\"},\n    \"fly\": { \"serverUrl\": \"https://mcp.fly.io\", \"headers\": {} }\n  },\n  \"keep\": true\n}\n")
	}

	return []byte("{\n  \"mcpServers\": {\n    \"other\": {\"command\": \"keep\"},\n    \"fly\": { \"url\": \"https://mcp.fly.io\", \"type\": \"http\" }\n  },\n  \"keep\": true\n}\n")
}

func differentJSON(client mcpClient) []byte {
	if client.slug == "opencode" {
		return []byte("{\n  \"mcp\": {\n    \"fly\": {\"type\": \"remote\", \"url\": \"https://example.invalid\"},\n    \"other\": {\"type\": \"remote\", \"url\": \"https://example.com\"}\n  },\n  \"keep\": true\n}\n")
	}
	if client.slug == "antigravity" {
		return []byte("{\n  \"mcpServers\": {\n    \"fly\": {\"serverUrl\": \"https://example.invalid\"},\n    \"other\": {\"serverUrl\": \"https://example.com\"}\n  },\n  \"keep\": true\n}\n")
	}

	return []byte("{\n  \"mcpServers\": {\n    \"fly\": {\"type\": \"http\", \"url\": \"https://example.invalid\"},\n    \"other\": {\"command\": \"keep\"}\n  },\n  \"keep\": true\n}\n")
}

func assertPreservedAround(t *testing.T, original, got []byte, snippet string) {
	t.Helper()
	idx := bytes.Index(original, []byte(snippet))
	require.GreaterOrEqual(t, idx, 0, "fixture missing snippet %s", snippet)
	end := idx + len(snippet)
	assert.Equal(t, original[:end], got[:end], "bytes before the insertion changed")
	assert.True(t, bytes.HasSuffix(got, original[end:]), "bytes after the insertion changed")
	assert.Greater(t, len(got), len(original))
}

func assertFlyPointsAt(t *testing.T, client mcpClient, data []byte, want string) {
	t.Helper()
	var root map[string]any
	require.NoError(t, json.Unmarshal(data, &root))
	container := root
	keys := client.container
	if client.slug == "opencode" {
		if mcp, ok := root["mcp"].(map[string]any); ok {
			if servers, ok := mcp["servers"].(map[string]any); ok {
				container = servers
				keys = nil
			} else {
				container = mcp
				keys = nil
			}
		}
	}
	for _, key := range keys {
		next, ok := container[key].(map[string]any)
		require.True(t, ok, "missing %s in %s", key, string(data))
		container = next
	}
	fly, ok := container["fly"].(map[string]any)
	require.True(t, ok, "missing fly entry in %s", string(data))
	got, _ := fly["url"].(string)
	if got == "" {
		got, _ = fly["serverUrl"].(string)
	}
	assert.Equal(t, want, got)
	for key := range fly {
		switch key {
		case "type", "url", "serverUrl", "tools", "enabled", "disabled":
		default:
			t.Errorf("unexpected fly config key %q", key)
		}
	}
}

func assertNoSecrets(t *testing.T, data []byte) {
	t.Helper()
	lower := bytes.ToLower(data)
	for _, banned := range []string{
		"token", "secret", "bearer", "authorization", "client_id", "client_secret",
		"password", "api_key", "fo1_", "flyv1",
	} {
		assert.NotContains(t, string(lower), banned)
	}
}

func writeMode(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, mode))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return data
}
