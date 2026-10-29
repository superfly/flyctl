package setup

import (
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// errManualEdit means the fly server is present but not as a standalone
// [mcp_servers.fly] table. The file parses; it just cannot be edited safely.
var errManualEdit = errors.New("can't safely edit; edit manually")

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

	table, present := codexFlyTable(root)
	if present && codexReady(table) {
		return nil, statusAlready, nil
	}
	if present && !force {
		return nil, statusConflict, nil
	}

	var next []byte
	if present {
		replaced, err := updateCodexSection(original)
		if err != nil {
			return nil, "", err
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

func codexFlyTable(root map[string]any) (map[string]any, bool) {
	servers, ok := root["mcp_servers"].(map[string]any)
	if !ok {
		return nil, false
	}
	entry, exists := servers[mcpServerName]
	if !exists {
		return nil, false
	}
	table, ok := entry.(map[string]any)
	if !ok {
		// Present, but not a table this command can treat as configured.
		return map[string]any{}, true
	}

	return table, true
}

// codexReady reports whether the fly table matches the shape this command
// writes. A matching URL with enabled = false is a conflict: the entry is
// disabled. An absent enabled key is fine, matching the JSON clients.
func codexReady(table map[string]any) bool {
	url, _ := table["url"].(string)
	if !urlsMatch(url, hostedMCPURL) {
		return false
	}
	if enabled, ok := table["enabled"].(bool); ok && !enabled {
		return false
	}

	return true
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

// updateCodexSection rewrites url lines and enabled = false inside the
// standalone [mcp_servers.fly] table and leaves every other line, including
// extra keys, comments and the following tables. An inline table is not a
// section this can cut safely, so it returns errManualEdit instead of a
// parse error.
func updateCodexSection(original []byte) ([]byte, error) {
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
		return nil, errManualEdit
	}

	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if anyHeaderRe.MatchString(strings.TrimRight(lines[i], "\r\n")) {
			end = i
			break
		}
	}

	var b strings.Builder
	for i := 0; i <= start; i++ {
		b.WriteString(lines[i])
	}
	if !strings.HasSuffix(lines[start], "\n") {
		b.WriteString(nl)
	}
	sawURL := false
	for i := start + 1; i < end; i++ {
		switch {
		case isCodexURLLine(lines[i]):
			b.WriteString(rewriteCodexURLLine(lines[i], nl))
			sawURL = true
		case isCodexEnabledFalseLine(lines[i]):
			b.WriteString(rewriteCodexEnabledLine(lines[i], nl))
		default:
			b.WriteString(lines[i])
		}
	}
	if !sawURL {
		b.WriteString("url = " + strconv.Quote(hostedMCPURL) + nl)
	}
	for i := end; i < len(lines); i++ {
		b.WriteString(lines[i])
	}

	return []byte(b.String()), nil
}

func isCodexURLLine(line string) bool {
	trimmed := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	key, _, ok := strings.Cut(trimmed, "=")

	return ok && strings.TrimSpace(key) == "url"
}

func isCodexEnabledFalseLine(line string) bool {
	trimmed := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return false
	}
	key, val, ok := strings.Cut(trimmed, "=")
	if !ok || strings.TrimSpace(key) != "enabled" {
		return false
	}
	val = strings.TrimSpace(val)
	if i := strings.Index(val, "#"); i >= 0 {
		val = strings.TrimSpace(val[:i])
	}

	return val == "false"
}

func rewriteCodexEnabledLine(line, nl string) string {
	return rewriteCodexAssignment(line, "enabled = true", nl)
}

func rewriteCodexURLLine(line, nl string) string {
	return rewriteCodexAssignment(line, "url = "+strconv.Quote(hostedMCPURL), nl)
}

func rewriteCodexAssignment(line, assignment, nl string) string {
	indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
	ending := ""
	body := line
	switch {
	case strings.HasSuffix(body, "\r\n"):
		ending = "\r\n"
		body = strings.TrimSuffix(body, "\r\n")
	case strings.HasSuffix(body, "\n"):
		ending = "\n"
		body = strings.TrimSuffix(body, "\n")
	}
	comment := ""
	if i := strings.Index(body, "#"); i >= 0 {
		comment = " " + strings.TrimSpace(body[i:])
	}
	if ending == "" {
		ending = nl
	}

	return indent + assignment + comment + ending
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
