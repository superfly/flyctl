package setup

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func TestJSONWriters(t *testing.T) {
	for _, client := range mcpClients {
		if client.format != formatJSON {
			continue
		}
		t.Run(client.slug, func(t *testing.T) {
			testFreshJSON(t, client)
			testPreserveOtherServers(t, client)
			testInsertMissingContainer(t, client)
			testIdenticalJSON(t, client)
			testConflictingJSON(t, client)
			testForceJSON(t, client)
			testUnparseableJSON(t, client)
			testJSONCCommentSurvives(t, client)
		})
	}
}

func testFreshJSON(t *testing.T, client mcpClient) {
	t.Helper()
	home := t.TempDir()
	out := configureMCP(home, client.slug, false, false)
	require.Equal(t, statusConfigured, out.Status)
	require.NoError(t, out.Err)

	path := client.path(home)
	data := readFile(t, path)
	assertNoSecrets(t, data)
	require.Equal(t, canonicalEntry(t, client), flyObject(t, path, client.container))
	if client.slug == "opencode" {
		var root map[string]any
		require.NoError(t, json.Unmarshal(data, &root))
		_, ok := root["$schema"]
		require.False(t, ok, "a fresh OpenCode config is the fly entry only")
	}

	info, err := os.Stat(path)
	require.NoError(t, err)
	if runtime.GOOS != "windows" {
		require.Equal(t, os.FileMode(0o644), info.Mode().Perm())
	}

	before := readFile(t, path)
	again := configureMCP(home, client.slug, false, false)
	require.Equal(t, statusAlready, again.Status)
	require.Equal(t, before, readFile(t, path))
}

func testPreserveOtherServers(t *testing.T, client mcpClient) {
	t.Helper()
	home := t.TempDir()
	path := client.path(home)
	original := []byte(`{"` + client.container + `":{"other":{"command":"echo","args":["keep-me"]}},"theme":"dark"}`)
	writeExisting(t, path, original, 0o640)

	out := configureMCP(home, client.slug, false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	assertInsertedOnly(t, original, next)
	require.Contains(t, string(next), `"other":{"command":"echo","args":["keep-me"]}`)
	require.Contains(t, string(next), `"theme":"dark"`)
	require.Equal(t, hostedMCPURL, flyObject(t, path, client.container)[client.urlKeys[0]])
	assertNoSecrets(t, next)

	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o640), info.Mode().Perm())
	}
}

func testInsertMissingContainer(t *testing.T, client mcpClient) {
	t.Helper()
	home := t.TempDir()
	path := client.path(home)
	original := []byte(`{"theme":"dark"}`)
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, client.slug, false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	require.True(t, bytes.HasPrefix(next, []byte(`{"theme":"dark"`)))
	require.True(t, bytes.HasSuffix(next, []byte(`}`)))
	require.Contains(t, string(next), `"`+client.container+`"`)
	require.Equal(t, hostedMCPURL, flyObject(t, path, client.container)[client.urlKeys[0]])
}

func testIdenticalJSON(t *testing.T, client mcpClient) {
	t.Helper()
	home := t.TempDir()
	path := client.path(home)
	entry := mustEntry(t, client)
	original := []byte(`{"` + client.container + `":{"fly":` + string(entry) + `,"other":{"command":"echo"}},"theme":"dark"}`)
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, client.slug, false, false)
	require.Equal(t, statusAlready, out.Status, out.Detail)
	require.Equal(t, original, readFile(t, path))
}

func testConflictingJSON(t *testing.T, client mcpClient) {
	t.Helper()
	home := t.TempDir()
	path := client.path(home)
	entry := bytes.ReplaceAll(mustEntry(t, client), []byte(hostedMCPURL), []byte("https://example.com"))
	original := []byte("{\n  // do not clobber\n  \"" + client.container + "\": {\"fly\": " + string(entry) + "}\n}\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, client.slug, false, false)
	require.Equal(t, statusConflict, out.Status, out.Detail)
	require.NoError(t, out.Err)
	require.Contains(t, out.Detail, "points elsewhere")
	require.Contains(t, out.Detail, "--force")
	require.Equal(t, original, readFile(t, path))
}

func testForceJSON(t *testing.T, client mcpClient) {
	t.Helper()
	home := t.TempDir()
	path := client.path(home)
	entry := bytes.ReplaceAll(mustEntry(t, client), []byte(hostedMCPURL), []byte("https://example.com"))
	original := []byte("{\n  // do not clobber\n  \"" + client.container + "\": {\"other\": {\"command\": \"echo\"}, \"fly\": " + string(entry) + "}\n}\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, client.slug, true, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	require.Contains(t, string(next), "// do not clobber")
	require.Contains(t, string(next), `"command": "echo"`)
	require.NotContains(t, string(next), "https://example.com")
	require.Equal(t, canonicalEntry(t, client), flyObject(t, path, client.container))
	assertNoSecrets(t, next)
	require.True(t, jsonEqual(mustEntry(t, client), mustMarshal(t, flyObject(t, path, client.container))))
}

func testUnparseableJSON(t *testing.T, client mcpClient) {
	t.Helper()
	home := t.TempDir()
	path := client.path(home)
	original := []byte("{")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, client.slug, true, false)
	require.Equal(t, statusFailed, out.Status)
	require.Error(t, out.Err)
	require.Contains(t, strings.ToLower(out.Err.Error()), "json")
	require.Equal(t, original, readFile(t, path))
}

func testJSONCCommentSurvives(t *testing.T, client mcpClient) {
	t.Helper()
	home := t.TempDir()
	path := client.path(home)
	original := []byte("{\n  // keep\n  \"theme\": \"dark\"\n}\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, client.slug, false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	require.Contains(t, string(next), "// keep")
	require.Contains(t, string(next), `"theme": "dark"`)
	require.Contains(t, string(next), hostedMCPURL)
}

func TestAntigravityURLCountsAsConfigured(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".gemini", "config", "mcp_config.json")
	original := []byte(`{"mcpServers":{"fly":{"url":"https://mcp.fly.io"}}}`)
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "antigravity", false, false)
	require.Equal(t, statusAlready, out.Status, out.Detail)
	require.Equal(t, original, readFile(t, path))
}

func TestCodexTOMLRoundTrip(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	original := []byte("# keep me\nmodel = \"gpt-5\"\n\n[profiles.ada]\nname = \"ada\"\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "codex", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	require.True(t, bytes.HasPrefix(next, original), "comments and other tables stay a byte prefix")
	assertCodex(t, next, hostedMCPURL, "gpt-5", "ada")
	assertNoSecrets(t, next)
	require.Contains(t, string(next), "# keep me")

	before := readFile(t, path)
	again := configureMCP(home, "codex", false, false)
	require.Equal(t, statusAlready, again.Status)
	require.Equal(t, before, readFile(t, path))
}

func TestCodexCRLFPreserved(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	original := []byte("# keep\r\nmodel = \"gpt-5\"\r\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "codex", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	require.True(t, bytes.HasPrefix(next, original))
	require.Contains(t, string(next), "\r\n")
	assertNoSecrets(t, next)
}

func TestCodexIdenticalNoOp(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	original := []byte("# keep\n\n[mcp_servers.fly]\nurl = \"https://mcp.fly.io\"\n\n[other]\nx = 1\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "codex", false, false)
	require.Equal(t, statusAlready, out.Status, out.Detail)
	require.Equal(t, original, readFile(t, path))
}

func TestCodexConflictRefused(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	original := []byte("# keep\n\n[mcp_servers.fly]\nurl = \"https://example.com\"\n\n[other]\nx = 1\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "codex", false, false)
	require.Equal(t, statusConflict, out.Status, out.Detail)
	require.Equal(t, original, readFile(t, path))
}

func TestCodexForceReplacesTableOnly(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	original := []byte("# keep me\n\n[mcp_servers.fly]\nurl = \"https://example.com\"\nnote = \"keep-me\"\n\n[profiles.ada]\n# comment stays\nname = \"ada\"\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "codex", true, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	require.Contains(t, string(next), "# keep me")
	require.Contains(t, string(next), "# comment stays")
	require.Contains(t, string(next), "name = \"ada\"")
	require.Contains(t, string(next), "note = \"keep-me\"")
	require.NotContains(t, string(next), "https://example.com")
	assertCodex(t, next, hostedMCPURL, "", "ada")
	assertNoSecrets(t, next)
}

func TestCodexInlineTableIsNotRewritten(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	original := []byte("[mcp_servers]\nfly = { url = \"https://example.com\" }\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "codex", true, false)
	require.Equal(t, statusFailed, out.Status)
	require.Error(t, out.Err)
	require.Contains(t, out.Err.Error(), "manually")
	require.NotContains(t, strings.ToLower(out.Err.Error()), "could not parse")
	require.NotContains(t, strings.ToLower(out.Err.Error()), "unparseable")
	require.Equal(t, original, readFile(t, path))
}

func TestCodexUnparseableUntouched(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".codex", "config.toml")
	original := []byte("= =\n")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "codex", true, false)
	require.Equal(t, statusFailed, out.Status)
	require.Error(t, out.Err)
	require.Contains(t, strings.ToLower(out.Err.Error()), "toml")
	require.Equal(t, original, readFile(t, path))
}

func TestCodexFreshFile(t *testing.T) {
	home := t.TempDir()
	out := configureMCP(home, "codex", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	data := readFile(t, filepath.Join(home, ".codex", "config.toml"))
	assertCodex(t, data, hostedMCPURL, "", "")
	assertNoSecrets(t, data)

	var root map[string]any
	require.NoError(t, toml.Unmarshal(data, &root))
	servers := root["mcp_servers"].(map[string]any)
	flyTable := servers["fly"].(map[string]any)
	require.Equal(t, map[string]any{"url": hostedMCPURL}, flyTable)
}

func TestSkillsOnlyAgentsWriteNoMCPFile(t *testing.T) {
	home := t.TempDir()
	for _, slug := range []string{"pi", "universal"} {
		out := configureMCP(home, slug, false, false)
		require.Equal(t, statusSkipped, out.Status)
		require.NoError(t, out.Err)
	}
	require.Contains(t, configureMCP(home, "pi", false, false).Detail, "no stable global MCP config")
	require.Contains(t, configureMCP(home, "universal", false, false).Detail, "no MCP configuration for this agent")
	_, err := os.Stat(filepath.Join(home, ".pi", "agent", "mcp.json"))
	require.True(t, os.IsNotExist(err))
}

func TestSkipMCPWritesNothing(t *testing.T) {
	home := t.TempDir()
	out := configureMCP(home, "claude-code", false, true)
	require.Equal(t, statusSkipped, out.Status)
	require.Contains(t, out.Detail, "--skip-mcp")
	_, err := os.Stat(filepath.Join(home, ".claude.json"))
	require.True(t, os.IsNotExist(err))
}

func TestTrailingCommaJSON(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	original := []byte("{\"mcpServers\":{\"other\":{\"command\":\"echo\"},},\"theme\":\"dark\"}")
	writeExisting(t, path, original, 0o644)

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	next := readFile(t, path)
	require.Contains(t, string(next), `"other":{"command":"echo"}`)
	require.Contains(t, string(next), `"theme":"dark"`)
	require.Equal(t, hostedMCPURL, flyObject(t, path, "mcpServers")["url"])
}

func TestEmptyFileIsCreated(t *testing.T) {
	home := t.TempDir()
	path := filepath.Join(home, ".claude.json")
	writeExisting(t, path, []byte(" \n"), 0o600)

	out := configureMCP(home, "claude-code", false, false)
	require.Equal(t, statusConfigured, out.Status, out.Detail)
	require.Equal(t, hostedMCPURL, flyObject(t, path, "mcpServers")["url"])
}

func assertInsertedOnly(t *testing.T, original, next []byte) {
	t.Helper()
	marker := []byte(`,"theme":"dark"}`)
	require.True(t, bytes.HasSuffix(original, marker))
	suffix := append([]byte("}"), marker...)
	require.True(t, bytes.HasSuffix(original, suffix))
	prefix := original[:len(original)-len(suffix)]
	require.True(t, bytes.HasPrefix(next, prefix), "bytes before the insertion changed")
	require.True(t, bytes.HasSuffix(next, suffix), "bytes after the insertion changed")
	inserted := next[len(prefix) : len(next)-len(suffix)]
	require.Contains(t, string(inserted), `"fly"`)
	require.Contains(t, string(inserted), hostedMCPURL)
	require.NotContains(t, string(inserted), "theme")
}

func assertCodex(t *testing.T, data []byte, flyURL, model, ada string) {
	t.Helper()
	var root map[string]any
	require.NoError(t, toml.Unmarshal(data, &root))
	servers, ok := root["mcp_servers"].(map[string]any)
	require.True(t, ok)
	entry, ok := servers["fly"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, flyURL, entry["url"])
	if model != "" {
		require.Equal(t, model, root["model"])
	}
	if ada != "" {
		profiles, ok := root["profiles"].(map[string]any)
		require.True(t, ok)
		person, ok := profiles["ada"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, ada, person["name"])
	}
}

func canonicalEntry(t *testing.T, client mcpClient) map[string]any {
	t.Helper()
	var obj map[string]any
	require.NoError(t, json.Unmarshal(mustEntry(t, client), &obj))

	return obj
}

func mustEntry(t *testing.T, client mcpClient) []byte {
	t.Helper()
	entry, err := client.entryJSON()
	require.NoError(t, err)
	assertNoSecrets(t, entry)

	return entry
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)

	return data
}

func flyObject(t *testing.T, path, container string) map[string]any {
	t.Helper()
	entry, err := flyEntryFromFile(path, container)
	require.NoError(t, err)

	return entry
}

func writeExisting(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, mode))
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Chmod(path, mode))
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	return data
}

func assertNoSecrets(t *testing.T, data []byte) {
	t.Helper()
	lower := strings.ToLower(string(data))
	for _, marker := range []string{
		"token",
		"secret",
		"authorization",
		"client_id",
		"client_secret",
		"bearer",
		"password",
		"api_key",
		"fo1_",
		"flyv1",
	} {
		require.NotContains(t, lower, marker)
	}
}
