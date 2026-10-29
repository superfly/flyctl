package setup

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

const codexTableHeader = "[mcp_servers.fly]"

const codexTableBody = "url = \"https://mcp.fly.io\"\n"

// dottedFlyURL matches a single-line `mcp_servers.fly.url = "..."` assignment
// so a file that does not use a [mcp_servers.fly] table can still be updated
// without rewriting the rest of it.
var dottedFlyURL = regexp.MustCompile(`^(\s*mcp_servers\.fly\.url\s*=\s*)(?:"(?:\\.|[^"])*"|'(?:\\.|[^'])*')(\s*(?:#.*)?)(\r?)$`)

// mergeTOMLBytes inserts or replaces [mcp_servers.fly] in a Codex config.
// Comments and every other table stay byte for byte. A document that does
// not parse is an error and nothing is returned to write.
func mergeTOMLBytes(src []byte, force bool) (mergeResult, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return mergeResult{status: mergeWrote, data: []byte(codexTableHeader + "\n" + codexTableBody)}, nil
	}

	var doc map[string]any
	if err := toml.Unmarshal(src, &doc); err != nil {
		return mergeResult{}, fmt.Errorf("could not parse TOML: %w", err)
	}

	url, exists, err := codexFlyURL(doc)
	if err != nil {
		return mergeResult{}, err
	}
	if exists && url == mcpURL {
		return mergeResult{status: mergeAlready, detail: "already configured"}, nil
	}
	if exists && !force {
		detail := fmt.Sprintf("fly entry does not point at %s", mcpURL)
		if url != "" {
			detail = fmt.Sprintf("fly entry points at %s", url)
		}

		return mergeResult{status: mergeRefused, detail: detail + "; re-run with --force to replace it"}, nil
	}

	if exists {
		if updated, ok := replaceCodexTable(src); ok {
			if err := tomlRoundTrip(updated); err != nil {
				return mergeResult{}, err
			}

			return mergeResult{status: mergeWrote, data: updated}, nil
		}
		if updated, ok := replaceDottedFlyURL(src); ok {
			if err := tomlRoundTrip(updated); err != nil {
				return mergeResult{}, err
			}

			return mergeResult{status: mergeWrote, data: updated}, nil
		}

		return mergeResult{}, fmt.Errorf("mcp_servers.fly is not a [mcp_servers.fly] table or a mcp_servers.fly.url assignment, so it was left unchanged")
	}

	out := appendCodexTable(src)
	if err := tomlRoundTrip(out); err != nil {
		return mergeResult{}, err
	}

	return mergeResult{status: mergeWrote, data: out}, nil
}

func tomlRoundTrip(b []byte) error {
	var doc map[string]any
	if err := toml.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("internal error: merge produced invalid TOML: %w", err)
	}
	url, exists, err := codexFlyURL(doc)
	if err != nil {
		return err
	}
	if !exists || url != mcpURL {
		return fmt.Errorf("internal error: merge did not set mcp_servers.fly.url")
	}

	return nil
}

func codexFlyURL(doc map[string]any) (url string, exists bool, err error) {
	raw, ok := doc["mcp_servers"]
	if !ok || raw == nil {
		return "", false, nil
	}
	servers, ok := raw.(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("mcp_servers is not a TOML table")
	}
	flyRaw, ok := servers["fly"]
	if !ok {
		return "", false, nil
	}
	fly, ok := flyRaw.(map[string]any)
	if !ok {
		return "", false, fmt.Errorf("mcp_servers.fly is not a TOML table")
	}
	url, _ = fly["url"].(string)

	return url, true, nil
}

func appendCodexTable(src []byte) []byte {
	out := append([]byte{}, src...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	if len(bytes.TrimSpace(src)) > 0 {
		out = append(out, '\n')
	}
	out = append(out, codexTableHeader...)
	out = append(out, '\n')
	out = append(out, codexTableBody...)

	return out
}

// replaceCodexTable replaces the body of [mcp_servers.fly], from the end of
// its header line up to the next table header. The header and every other
// line stay as they were.
func replaceCodexTable(src []byte) ([]byte, bool) {
	lines := splitKeepEnds(src)
	for i, line := range lines {
		if !isFlyTableHeader(line) {
			continue
		}
		bodyEnd := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if isTableHeader(lines[j]) {
				bodyEnd = j
				break
			}
		}
		var b bytes.Buffer
		for _, l := range lines[:i+1] {
			b.WriteString(l)
		}
		if !strings.HasSuffix(lines[i], "\n") && bodyEnd == len(lines) {
			b.WriteByte('\n')
		}
		b.WriteString(codexTableBody)
		for _, l := range lines[bodyEnd:] {
			b.WriteString(l)
		}

		return b.Bytes(), true
	}

	return nil, false
}

func replaceDottedFlyURL(src []byte) ([]byte, bool) {
	lines := splitKeepEnds(src)
	for i, line := range lines {
		nl := ""
		body := line
		if strings.HasSuffix(body, "\n") {
			body = strings.TrimSuffix(body, "\n")
			nl = "\n"
		}
		if !dottedFlyURL.MatchString(body) {
			continue
		}
		lines[i] = dottedFlyURL.ReplaceAllString(body, `${1}"`+mcpURL+`"${2}`+nl)
		var b bytes.Buffer
		for _, l := range lines {
			b.WriteString(l)
		}

		return b.Bytes(), true
	}

	return nil, false
}

func splitKeepEnds(src []byte) []string {
	text := string(src)
	if text == "" {
		return nil
	}
	parts := strings.SplitAfter(text, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}

	return parts
}

func isFlyTableHeader(line string) bool {
	trim := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
	if strings.HasPrefix(trim, "#") || !strings.HasPrefix(trim, codexTableHeader) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trim, codexTableHeader))

	return rest == "" || strings.HasPrefix(rest, "#")
}

func isTableHeader(line string) bool {
	trim := strings.TrimSpace(strings.TrimRight(line, "\r\n"))
	if trim == "" || strings.HasPrefix(trim, "#") {
		return false
	}

	return strings.HasPrefix(trim, "[")
}
