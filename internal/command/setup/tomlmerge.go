package setup

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"sort"
	"strconv"

	"github.com/pelletier/go-toml/v2"
	"github.com/pelletier/go-toml/v2/unstable"
)

// errManualEdit means the fly server is present but cannot be edited
// safely: it is not a standalone [mcp_servers.fly] table, it is a stdio
// entry, or the rewrite would not produce the expected document. The file
// parses; it just has to be fixed by hand.
var errManualEdit = errors.New("can't safely edit; edit manually")

// stdioKeys mark an entry that launches a local process. Overlaying the
// hosted URL onto one would leave a hybrid entry, so such entries are never
// rewritten.
var stdioKeys = []string{"command", "args", "env"}

func hasStdioKeys(obj map[string]any) bool {
	for _, key := range stdioKeys {
		if _, ok := obj[key]; ok {
			return true
		}
	}

	return false
}

// mergeCodexBytes inserts [mcp_servers.fly] into a Codex config.
//
// The original bytes are kept as a prefix and the table is appended, so
// comments and other tables stay intact. A file that does not parse is
// returned via a non-nil error and must not be written. out is nil when the
// file should not change. A leading UTF-8 BOM is stripped for the parse and
// put back on a write.
//
// With force, the url and enabled keys of an existing standalone table are
// rewritten in place using parser positions, so strings and comments that
// merely look like a table header or an assignment are never touched. The
// rewritten bytes are parsed again and compared with the original: anything
// other than the keys this command owns changing is an errManualEdit.
func mergeCodexBytes(original []byte, force bool) (out []byte, status string, err error) {
	bom, body := splitBOM(original)
	if len(bytes.TrimSpace(body)) == 0 {
		return withBOM(bom, []byte(codexTable("\n"))), statusConfigured, nil
	}

	var root map[string]any
	if err := toml.Unmarshal(body, &root); err != nil {
		return nil, "", fmt.Errorf("not valid TOML: %w", err)
	}

	table, present := codexFlyTable(root)
	if present && codexReady(table) {
		return nil, statusAlready, nil
	}
	if present && !force {
		return nil, statusConflict, nil
	}
	if present && hasStdioKeys(table) {
		return nil, "", fmt.Errorf("%w: [mcp_servers.fly] is a stdio server", errManualEdit)
	}

	var next []byte
	if present {
		next, err = updateCodexSection(body)
		if err != nil {
			return nil, "", err
		}
	} else {
		next = appendCodexTable(body)
	}

	if err := verifyCodexEdit(root, next); err != nil {
		return nil, "", err
	}

	return withBOM(bom, next), statusConfigured, nil
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
	return "[mcp_servers." + mcpServerName + "]" + nl + codexURLAssignment() + nl
}

func codexURLAssignment() string {
	return "url = " + strconv.Quote(hostedMCPURL)
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

// verifyCodexEdit parses the rewritten bytes and checks that the fly table
// now points at the hosted server and that nothing else changed. A rewrite
// that fails this check is reported as errManualEdit: the original parsed,
// so the problem is the edit, not the file.
func verifyCodexEdit(before map[string]any, next []byte) error {
	var after map[string]any
	if err := toml.Unmarshal(next, &after); err != nil {
		return fmt.Errorf("%w: the rewritten config does not parse: %v", errManualEdit, err)
	}
	table, present := codexFlyTable(after)
	if !present || !codexReady(table) {
		return fmt.Errorf("%w: the rewrite did not produce a hosted [mcp_servers.fly] table", errManualEdit)
	}
	if !reflect.DeepEqual(codexComparable(before), codexComparable(after)) {
		return fmt.Errorf("%w: the rewrite would change keys outside [mcp_servers.fly]", errManualEdit)
	}

	return nil
}

// codexComparable copies root without the keys this command owns, so the
// parse before and after an edit can be compared directly.
func codexComparable(root map[string]any) map[string]any {
	out := maps.Clone(root)
	servers, ok := out["mcp_servers"].(map[string]any)
	if !ok {
		return out
	}
	servers = maps.Clone(servers)
	out["mcp_servers"] = servers
	if fly, ok := servers[mcpServerName].(map[string]any); ok {
		fly = maps.Clone(fly)
		delete(fly, "url")
		delete(fly, "enabled")
		if len(fly) == 0 {
			delete(servers, mcpServerName)
		} else {
			servers[mcpServerName] = fly
		}
	}
	if len(servers) == 0 {
		delete(out, "mcp_servers")
	}

	return out
}

// codexSection is the standalone [mcp_servers.fly] table located by the
// parser. bodyStart is the offset just past the header line.
type codexSection struct {
	bodyStart int
	keyvals   []codexKeyval
}

// codexKeyval is one simple `key = value` inside the fly table. start and
// end bound the key through the end of the value; a trailing comment is
// outside the range.
type codexKeyval struct {
	key        string
	start, end int
	isFalse    bool
}

// updateCodexSection rewrites url and enabled = false inside the standalone
// [mcp_servers.fly] table and leaves every other byte, including extra keys,
// comments and the following tables. A fly entry defined any other way (an
// inline table, a dotted key under [mcp_servers]) is errManualEdit.
func updateCodexSection(body []byte) ([]byte, error) {
	section, err := locateCodexSection(body)
	if err != nil {
		return nil, err
	}
	nl := lineEnding(body)

	type edit struct {
		start, end int
		text       string
	}
	var edits []edit
	sawURL := false
	for _, kv := range section.keyvals {
		switch kv.key {
		case "url":
			sawURL = true
			edits = append(edits, edit{kv.start, kv.end, codexURLAssignment()})
		case "enabled":
			if kv.isFalse {
				edits = append(edits, edit{kv.start, kv.end, "enabled = true"})
			}
		}
	}
	if !sawURL {
		text := codexURLAssignment() + nl
		if section.bodyStart > 0 && body[section.bodyStart-1] != '\n' {
			text = nl + text
		}
		edits = append(edits, edit{section.bodyStart, section.bodyStart, text})
	}
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].start > edits[j].start
	})

	out := body
	for _, e := range edits {
		out, err = splice(out, e.start, e.end, []byte(e.text))
		if err != nil {
			return nil, err
		}
	}

	return out, nil
}

// locateCodexSection walks the top-level expressions and returns the
// standalone [mcp_servers.fly] table. Strings, comments and other tables
// are skipped by the parser, so a header-looking line inside a multi-line
// string is never mistaken for the table.
func locateCodexSection(body []byte) (*codexSection, error) {
	parser := &unstable.Parser{}
	parser.Reset(body)

	var section *codexSection
	inFly := false
	for parser.NextExpression() {
		node := parser.Expression()
		switch node.Kind {
		case unstable.Table, unstable.ArrayTable:
			if inFly {
				return section, nil
			}
			keys := keyParts(node)
			inFly = node.Kind == unstable.Table && len(keys) == 2 && keys[0] == "mcp_servers" && keys[1] == mcpServerName
			if !inFly {
				continue
			}
			end := 0
			for it := node.Key(); it.Next(); {
				end = int(it.Node().Raw.Offset + it.Node().Raw.Length)
			}
			if i := bytes.IndexByte(body[end:], '\n'); i >= 0 {
				end += i + 1
			} else {
				end = len(body)
			}
			section = &codexSection{bodyStart: end}
		case unstable.KeyValue:
			if !inFly {
				continue
			}
			keys := keyParts(node)
			if len(keys) != 1 {
				continue
			}
			value := node.Value()
			section.keyvals = append(section.keyvals, codexKeyval{
				key:     keys[0],
				start:   int(node.Raw.Offset),
				end:     int(node.Raw.Offset + node.Raw.Length),
				isFalse: value.Kind == unstable.Bool && string(value.Data) == "false",
			})
		}
	}
	if err := parser.Error(); err != nil {
		return nil, fmt.Errorf("%w: %v", errManualEdit, err)
	}
	if section == nil {
		return nil, errManualEdit
	}

	return section, nil
}

func keyParts(node *unstable.Node) []string {
	var parts []string
	for it := node.Key(); it.Next(); {
		parts = append(parts, string(it.Node().Data))
	}

	return parts
}
