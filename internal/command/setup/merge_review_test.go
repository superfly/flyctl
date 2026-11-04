package setup

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/require"
)

func mustClient(t *testing.T, slug string) mcpClient {
	t.Helper()
	client, ok := clientFor(slug)
	require.True(t, ok)

	return client
}

func codexFlyURL(t *testing.T, data []byte) string {
	t.Helper()
	var root map[string]any
	require.NoError(t, toml.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &root))
	table, present := codexFlyTable(root)
	require.True(t, present)
	url, _ := table["url"].(string)

	return url
}

// Finding 1: a header-looking line inside a multi-line string must not be
// mistaken for the table.
func TestCodexForceIgnoresHeaderInsideString(t *testing.T) {
	original := []byte("notes = \"\"\"\n[mcp_servers.fly]\nurl = \"decoy\"\n\"\"\"\n\n[mcp_servers.fly]\nurl = \"https://example.com\"\n")

	out, status, err := mergeCodexBytes(original, true)
	require.NoError(t, err)
	require.Equal(t, statusConfigured, status)
	require.Equal(t, hostedMCPURL, codexFlyURL(t, out))
	require.NotContains(t, string(out), "https://example.com")
	require.Contains(t, string(out), "notes = \"\"\"\n[mcp_servers.fly]\nurl = \"decoy\"\n\"\"\"\n", "the string is left alone")

	var root map[string]any
	require.NoError(t, toml.Unmarshal(out, &root))
	require.Equal(t, "[mcp_servers.fly]\nurl = \"decoy\"\n", root["notes"])
}

// Finding 2: --force never overlays the hosted URL onto a stdio entry.
func TestForceRefusesStdioEntry(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		original := []byte(`{"mcpServers":{"fly":{"type":"stdio","command":"npx","args":["x"]}}}`)
		out, status, err := mergeJSONBytes(original, mustClient(t, "claude-code"), true)
		require.True(t, errors.Is(err, errManualEdit), "got %v", err)
		require.Contains(t, err.Error(), "edit manually")
		require.Nil(t, out)
		require.Empty(t, status)

		out, status, err = mergeJSONBytes(original, mustClient(t, "claude-code"), false)
		require.NoError(t, err)
		require.Equal(t, statusConflict, status)
		require.Nil(t, out)
	})

	t.Run("toml", func(t *testing.T) {
		original := []byte("[mcp_servers.fly]\ncommand = \"npx\"\nargs = [\"x\"]\n")
		out, status, err := mergeCodexBytes(original, true)
		require.True(t, errors.Is(err, errManualEdit), "got %v", err)
		require.Contains(t, err.Error(), "edit manually")
		require.Nil(t, out)
		require.Empty(t, status)

		out, status, err = mergeCodexBytes(original, false)
		require.NoError(t, err)
		require.Equal(t, statusConflict, status)
		require.Nil(t, out)
	})
}

// Finding 3: when the original parses but the rewrite would not, the caller
// must hear "edit manually", not "could not parse".
func TestCodexRewriteThatDoesNotParseIsManualEdit(t *testing.T) {
	for name, original := range map[string]string{
		"inline mcp_servers": "mcp_servers = { other = { url = \"x\" } }\n",
		"scalar mcp_servers": "mcp_servers = 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			out, status, err := mergeCodexBytes([]byte(original), true)
			require.True(t, errors.Is(err, errManualEdit), "got %v", err)
			require.NotContains(t, strings.ToLower(err.Error()), "not valid toml")
			require.Nil(t, out)
			require.Empty(t, status)
		})
	}
}

// A quoted key is still the url key; the parser positions cover it.
func TestCodexForceRewritesQuotedURLKey(t *testing.T) {
	original := []byte("[mcp_servers.fly]\n\"url\" = \"https://example.com\"\n")

	out, status, err := mergeCodexBytes(original, true)
	require.NoError(t, err)
	require.Equal(t, statusConfigured, status)
	require.Equal(t, hostedMCPURL, codexFlyURL(t, out))
	require.NotContains(t, string(out), "https://example.com")
}

func TestCodexForceAddsMissingURLAfterHeader(t *testing.T) {
	original := []byte("[mcp_servers.fly]\nenabled = false # off\n\n[other]\nx = 1\n")

	out, status, err := mergeCodexBytes(original, true)
	require.NoError(t, err)
	require.Equal(t, statusConfigured, status)
	require.Equal(t, "[mcp_servers.fly]\nurl = \"https://mcp.fly.io\"\nenabled = true # off\n\n[other]\nx = 1\n", string(out))
}

// Finding 4: a member inserted into a pretty-printed object follows the last
// member's comma and indentation instead of landing before the brace.
func TestInsertIntoPrettyObjectFollowsLastMember(t *testing.T) {
	client := mustClient(t, "claude-code")

	t.Run("nested container", func(t *testing.T) {
		original := "{\n  \"mcpServers\": {\n    \"a\": {\n      \"command\": \"x\"\n    }\n  },\n  \"theme\": \"dark\"\n}\n"
		want := "{\n  \"mcpServers\": {\n    \"a\": {\n      \"command\": \"x\"\n    },\n    \"fly\": {\n      \"type\": \"http\",\n      \"url\": \"https://mcp.fly.io\"\n    }\n  },\n  \"theme\": \"dark\"\n}\n"
		out, status, err := mergeJSONBytes([]byte(original), client, false)
		require.NoError(t, err)
		require.Equal(t, statusConfigured, status)
		require.Equal(t, want, string(out))
	})

	t.Run("root container missing", func(t *testing.T) {
		original := "{\n  \"theme\": \"dark\"\n}\n"
		want := "{\n  \"theme\": \"dark\",\n  \"mcpServers\": {\n    \"fly\": {\n      \"type\": \"http\",\n      \"url\": \"https://mcp.fly.io\"\n    }\n  }\n}\n"
		out, status, err := mergeJSONBytes([]byte(original), client, false)
		require.NoError(t, err)
		require.Equal(t, statusConfigured, status)
		require.Equal(t, want, string(out))
	})

	t.Run("trailing comma is reused", func(t *testing.T) {
		original := "{\n  \"mcpServers\": {\n    \"a\": {\n      \"command\": \"x\"\n    },\n  },\n}\n"
		want := "{\n  \"mcpServers\": {\n    \"a\": {\n      \"command\": \"x\"\n    },\n    \"fly\": {\n      \"type\": \"http\",\n      \"url\": \"https://mcp.fly.io\"\n    }\n  },\n}\n"
		out, status, err := mergeJSONBytes([]byte(original), client, false)
		require.NoError(t, err)
		require.Equal(t, statusConfigured, status)
		require.Equal(t, want, string(out))
	})

	t.Run("crlf", func(t *testing.T) {
		original := "{\r\n  \"mcpServers\": {\r\n    \"a\": {\r\n      \"command\": \"x\"\r\n    }\r\n  }\r\n}\r\n"
		want := "{\r\n  \"mcpServers\": {\r\n    \"a\": {\r\n      \"command\": \"x\"\r\n    },\r\n    \"fly\": {\r\n      \"type\": \"http\",\r\n      \"url\": \"https://mcp.fly.io\"\r\n    }\r\n  }\r\n}\r\n"
		out, status, err := mergeJSONBytes([]byte(original), client, false)
		require.NoError(t, err)
		require.Equal(t, statusConfigured, status)
		require.Equal(t, want, string(out))
	})

	t.Run("compact stays compact", func(t *testing.T) {
		original := `{"mcpServers":{"a":{"command":"x"}},"theme":"dark"}`
		want := `{"mcpServers":{"a":{"command":"x"},"fly": {"type":"http","url":"https://mcp.fly.io"}},"theme":"dark"}`
		out, status, err := mergeJSONBytes([]byte(original), client, false)
		require.NoError(t, err)
		require.Equal(t, statusConfigured, status)
		require.Equal(t, want, string(out))
	})

	t.Run("compact trailing comma", func(t *testing.T) {
		original := `{"mcpServers":{"a":{"command":"x"},}}`
		want := `{"mcpServers":{"a":{"command":"x"},"fly": {"type":"http","url":"https://mcp.fly.io"}}}`
		out, status, err := mergeJSONBytes([]byte(original), client, false)
		require.NoError(t, err)
		require.Equal(t, statusConfigured, status)
		require.Equal(t, want, string(out))
	})

	t.Run("empty object", func(t *testing.T) {
		original := `{"mcpServers":{}}`
		want := `{"mcpServers":{"fly": {"type":"http","url":"https://mcp.fly.io"}}}`
		out, status, err := mergeJSONBytes([]byte(original), client, false)
		require.NoError(t, err)
		require.Equal(t, statusConfigured, status)
		require.Equal(t, want, string(out))
	})
}

// Nit 5a: a # inside the quoted value is part of the URL, not a comment.
func TestCodexForceKeepsHashInsideURL(t *testing.T) {
	original := []byte("[mcp_servers.fly]\nurl = \"https://example.com/#x\" # note\n")

	out, status, err := mergeCodexBytes(original, true)
	require.NoError(t, err)
	require.Equal(t, statusConfigured, status)
	require.Equal(t, "[mcp_servers.fly]\nurl = \"https://mcp.fly.io\" # note\n", string(out))
}

// Nit 5b: a UTF-8 BOM is stripped for the parse and put back on the write.
func TestCodexBOMPreserved(t *testing.T) {
	original := append(append([]byte{}, utf8BOM...), []byte("model = \"gpt-5\"\n")...)

	out, status, err := mergeCodexBytes(original, false)
	require.NoError(t, err)
	require.Equal(t, statusConfigured, status)
	require.True(t, bytes.HasPrefix(out, utf8BOM))
	require.True(t, bytes.HasPrefix(out, original))
	require.Equal(t, hostedMCPURL, codexFlyURL(t, out))

	out, status, err = mergeCodexBytes(out, false)
	require.NoError(t, err)
	require.Equal(t, statusAlready, status)
	require.Nil(t, out)
}

// Nit 5c: userinfo or a fragment means the URL is not the hosted server.
func TestURLsMatchRejectsUserinfoAndFragment(t *testing.T) {
	for _, got := range []string{
		"https://user@mcp.fly.io",
		"https://user:pw@mcp.fly.io/",
		"https://mcp.fly.io#frag",
		"https://mcp.fly.io/#x",
	} {
		require.False(t, urlsMatch(got, hostedMCPURL), got)
	}
	require.True(t, urlsMatch("https://mcp.fly.io/", hostedMCPURL))
	require.True(t, urlsMatch("HTTPS://MCP.FLY.IO", hostedMCPURL))
}
