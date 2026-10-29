package setup

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

var (
	flyHeaderRe = regexp.MustCompile(`^\s*\[\s*mcp_servers\s*\.\s*fly\s*\]\s*(?:#.*)?$`)
	anyHeaderRe = regexp.MustCompile(`^\s*\[`)
)

// mergeCodexBytes inserts [mcp_servers.fly] into a Codex config.
//
// The original bytes are kept as a prefix and the table is appended, so
// comments and other tables stay intact. A file that does not parse is
// returned via a non-nil error and must not be written. out is nil when the
// file should not change.
func mergeCodexBytes(original []byte, force bool) (out []byte, status string, err error) {
	if len(bytes.TrimSpace(original)) == 0 {
		return []byte(codexTable("\n")), statusConfigured, nil
	}

	var root map[string]any
	if err := toml.Unmarshal(original, &root); err != nil {
		return nil, "", fmt.Errorf("not valid TOML: %w", err)
	}

	url, present := codexFlyURL(root)
	if present && url == hostedMCPURL {
		return nil, statusAlready, nil
	}
	if present && !force {
		return nil, statusConflict, nil
	}

	var next []byte
	if present {
		replaced, ok, err := replaceCodexTable(original)
		if err != nil {
			return nil, "", err
		}
		if !ok {
			return nil, "", fmt.Errorf("[mcp_servers.fly] in the Codex config is not a table this command can replace without rewriting the file")
		}
		next = replaced
	} else {
		next = appendCodexTable(original)
	}

	if err := toml.Unmarshal(next, new(map[string]any)); err != nil {
		return nil, "", fmt.Errorf("refusing to write TOML that does not parse: %w", err)
	}

	return next, statusConfigured, nil
}

func codexFlyURL(root map[string]any) (string, bool) {
	servers, ok := root["mcp_servers"].(map[string]any)
	if !ok {
		return "", false
	}
	entry, exists := servers[mcpServerName]
	if !exists {
		return "", false
	}
	table, ok := entry.(map[string]any)
	if !ok {
		return "", true
	}
	url, _ := table["url"].(string)

	return url, true
}

func codexTable(nl string) string {
	return "[mcp_servers." + mcpServerName + "]" + nl + "url = " + strconv.Quote(hostedMCPURL) + nl
}

func lineEnding(data []byte) string {
	if bytes.Contains(data, []byte("\r\n")) {
		return "\r\n"
	}

	return "\n"
}

// appendCodexTable leaves original as a byte-for-byte prefix when it already
// ends in a newline. A blank line separates the new table from what was there.
func appendCodexTable(original []byte) []byte {
	nl := lineEnding(original)
	out := append([]byte{}, original...)
	if !bytes.HasSuffix(out, []byte(nl)) {
		out = append(out, nl...)
	}
	if !bytes.HasSuffix(out, []byte(nl+nl)) {
		out = append(out, nl...)
	}

	return append(out, codexTable(nl)...)
}

// replaceCodexTable swaps the [mcp_servers.fly] section for the hosted entry
// and copies every other line through unchanged.
func replaceCodexTable(original []byte) ([]byte, bool, error) {
	nl := lineEnding(original)
	lines := splitLines(string(original))
	start := -1
	for i, line := range lines {
		if flyHeaderRe.MatchString(strings.TrimRight(line, "\r\n")) {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, false, nil
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if anyHeaderRe.MatchString(strings.TrimRight(lines[i], "\r\n")) {
			end = i
			break
		}
	}

	var b strings.Builder
	for i := 0; i < start; i++ {
		b.WriteString(lines[i])
	}
	if built := b.String(); built != "" && !strings.HasSuffix(built, nl) {
		b.WriteString(nl)
	}
	b.WriteString(codexTable(nl))
	for i := end; i < len(lines); i++ {
		b.WriteString(lines[i])
	}

	return []byte(b.String()), true, nil
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	var lines []string
	for text != "" {
		i := strings.IndexAny(text, "\r\n")
		if i < 0 {
			lines = append(lines, text)
			break
		}
		end := i + 1
		if text[i] == '\r' && end < len(text) && text[end] == '\n' {
			end++
		}
		lines = append(lines, text[:end])
		text = text[end:]
	}

	return lines
}
