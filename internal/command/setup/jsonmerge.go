package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

// mergeJSONBytes inserts the fly server into a JSON or JSONC document.
//
// Existing bytes are preserved apart from the inserted or updated member.
// The edit is a splice at an offset reported by the parser, not a
// re-serialize, so comments, key order and unrelated servers stay
// byte-for-byte. A document that does not parse is returned untouched via
// a non-nil error; callers must not write it.
//
// already_configured means every fly entry in the container the client
// actually reads matches the shape this command writes. Duplicate fly keys
// that disagree are a conflict. --force updates only the keys this command
// owns, on every copy, and leaves extra keys such as headers in place.
//
// out is nil when the file should not change (already configured, or a
// conflicting entry and force is false). A leading UTF-8 BOM is stripped
// for the parse and put back on a write.
func mergeJSONBytes(original []byte, client mcpClient, force bool) (out []byte, status string, err error) {
	entry, err := client.entryJSON()
	if err != nil {
		return nil, "", err
	}
	bom, body := splitBOM(original)

	root, err := hujson.Parse(body)
	if err != nil {
		return nil, "", fmt.Errorf("not valid JSON: %w", err)
	}
	if root.Value == nil || root.Value.Kind() != '{' {
		return nil, "", errors.New("not valid JSON: the file is not a JSON object")
	}

	container, v2, err := locateContainer(&root, client)
	if err != nil {
		return nil, "", err
	}
	desired := entry
	if v2 {
		desired = opencodeV2Entry()
	}

	if container == nil {
		wrapped := append(append([]byte(`{"`+mcpServerName+`":`), desired...), '}')
		edited, err := insertMember(body, &root, client.container, wrapped)
		if err != nil {
			return nil, "", err
		}
		if err := validateJSON(edited); err != nil {
			return nil, "", err
		}

		return withBOM(bom, edited), statusConfigured, nil
	}

	flies := membersNamed(container, mcpServerName)
	if len(flies) == 0 {
		edited, err := insertMember(body, container, mcpServerName, desired)
		if err != nil {
			return nil, "", err
		}
		if err := validateJSON(edited); err != nil {
			return nil, "", err
		}

		return withBOM(bom, edited), statusConfigured, nil
	}

	for _, fly := range flies {
		if entryMatches(body, fly, client) {
			continue
		}
		if !force {
			return nil, statusConflict, nil
		}
		edited, status, err := repairFlies(body, flies, client, v2, desired)
		if err != nil {
			return nil, "", err
		}

		return withBOM(bom, edited), status, nil
	}

	return nil, statusAlready, nil
}

// repairFlies overlays owned keys onto every fly value. Splices run from
// the highest offset down so earlier offsets stay valid.
func repairFlies(body []byte, flies []*hujson.Value, client mcpClient, v2 bool, desired []byte) ([]byte, string, error) {
	type span struct {
		start, end int
		raw        []byte
	}
	spans := make([]span, 0, len(flies))
	for _, fly := range flies {
		raw, err := overlayRaw(body, fly, client, v2, desired)
		if err != nil {
			return nil, "", err
		}
		spans = append(spans, span{start: fly.StartOffset, end: fly.EndOffset, raw: raw})
	}
	sort.Slice(spans, func(i, j int) bool {
		return spans[i].start > spans[j].start
	})

	edited := body
	for _, sp := range spans {
		var err error
		edited, err = splice(edited, sp.start, sp.end, sp.raw)
		if err != nil {
			return nil, "", err
		}
	}
	if err := validateJSON(edited); err != nil {
		return nil, "", err
	}

	return edited, statusConfigured, nil
}

// locateContainer returns the object that holds server entries.
// OpenCode v2 keeps them under mcp.servers; a file that already has that
// object is updated there and nowhere else. The last duplicate container
// is the one clients read.
func locateContainer(root *hujson.Value, client mcpClient) (container *hujson.Value, v2 bool, err error) {
	if client.slug == "opencode" {
		mcp := lastMember(root, "mcp")
		if mcp == nil {
			return nil, false, nil
		}
		if !isObject(mcp) {
			return nil, false, errors.New("mcp is not a JSON object")
		}
		servers := lastMember(mcp, "servers")
		if servers == nil {
			return mcp, false, nil
		}
		if !isObject(servers) {
			return nil, false, errors.New("mcp.servers is not a JSON object")
		}

		return servers, true, nil
	}

	container = lastMember(root, client.container)
	if container == nil {
		return nil, false, nil
	}
	if !isObject(container) {
		return nil, false, fmt.Errorf("%s is not a JSON object", client.container)
	}

	return container, false, nil
}

func opencodeV2Entry() []byte {
	return mustCompact(map[string]string{
		"type": "remote",
		"url":  hostedMCPURL,
	})
}

// entryMatches reports whether one fly value is the shape this command
// writes. Extra keys do not by themselves make it a conflict. A trailing
// slash on the hosted URL does not either.
func entryMatches(body []byte, value *hujson.Value, client mcpClient) bool {
	obj, ok := objectMap(valueBytes(body, value))
	if !ok {
		return false
	}
	if disabled, isBool := obj["disabled"].(bool); isBool && disabled {
		return false
	}
	if enabled, isBool := obj["enabled"].(bool); isBool && !enabled {
		return false
	}
	if client.slug == "antigravity" {
		return antigravityURLsMatch(obj)
	}
	if !urlsMatch(stringField(obj, "url"), hostedMCPURL) {
		return false
	}
	switch client.slug {
	case "claude-code", "copilot", "factory-droid":
		if obj["type"] != "http" {
			return false
		}
	case "opencode":
		if obj["type"] != "remote" {
			return false
		}
	}
	if client.slug == "copilot" {
		tools, isList := obj["tools"].([]any)
		if !isList || len(tools) != 1 || tools[0] != "*" {
			return false
		}
	}

	return true
}

func antigravityURLsMatch(obj map[string]any) bool {
	_, hasServer := obj["serverUrl"]
	_, hasURL := obj["url"]
	switch {
	case hasServer && hasURL:
		return urlsMatch(stringField(obj, "serverUrl"), hostedMCPURL) && urlsMatch(stringField(obj, "url"), hostedMCPURL)
	case hasServer:
		return urlsMatch(stringField(obj, "serverUrl"), hostedMCPURL)
	case hasURL:
		return urlsMatch(stringField(obj, "url"), hostedMCPURL)
	default:
		return false
	}
}

// urlsMatch compares scheme, userinfo, host, path, query and fragment. A
// trailing slash on the path is ignored. A different host, path, query,
// http versus https, or any userinfo or fragment the hosted URL lacks is
// not a match.
func urlsMatch(got, want string) bool {
	if got == want {
		return true
	}
	left, errL := url.Parse(got)
	right, errR := url.Parse(want)
	if errL != nil || errR != nil || left.Host == "" || right.Host == "" {
		return strings.TrimRight(got, "/") == strings.TrimRight(want, "/")
	}

	return strings.EqualFold(left.Scheme, right.Scheme) &&
		left.User.String() == right.User.String() &&
		strings.EqualFold(left.Host, right.Host) &&
		strings.TrimRight(left.Path, "/") == strings.TrimRight(right.Path, "/") &&
		left.RawQuery == right.RawQuery &&
		left.Fragment == right.Fragment
}

func stringField(obj map[string]any, key string) string {
	value, _ := obj[key].(string)

	return value
}

// overlayRaw updates the keys this command owns and marshals that value
// back. The surrounding document is spliced around it, so unrelated keys
// and servers stay put. Comments inside the fly value itself are not kept.
// A fly value that is not an object is replaced by the canonical entry. A
// stdio entry (command, args or env) is never overlaid: the result would be
// a hybrid that is neither a local nor a hosted server, so it is
// errManualEdit instead.
func overlayRaw(body []byte, value *hujson.Value, client mcpClient, v2 bool, desired []byte) ([]byte, error) {
	obj, ok := objectMap(valueBytes(body, value))
	if !ok {
		return desired, nil
	}
	if hasStdioKeys(obj) {
		return nil, fmt.Errorf("%w: %s is a stdio server", errManualEdit, mcpServerName)
	}
	if disabled, isBool := obj["disabled"].(bool); isBool && disabled {
		obj["disabled"] = false
	}
	if enabled, isBool := obj["enabled"].(bool); isBool && !enabled {
		obj["enabled"] = true
	}
	switch client.slug {
	case "claude-code":
		obj["type"] = "http"
		obj["url"] = hostedMCPURL
	case "cursor":
		obj["url"] = hostedMCPURL
	case "copilot":
		obj["type"] = "http"
		obj["url"] = hostedMCPURL
		obj["tools"] = []any{"*"}
	case "factory-droid":
		obj["type"] = "http"
		obj["url"] = hostedMCPURL
		obj["disabled"] = false
	case "opencode":
		obj["type"] = "remote"
		obj["url"] = hostedMCPURL
		if !v2 {
			obj["enabled"] = true
		}
	case "antigravity":
		obj["serverUrl"] = hostedMCPURL
		if _, exists := obj["url"]; exists {
			obj["url"] = hostedMCPURL
		}
	}

	return marshalValue(obj)
}

func marshalValue(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}

	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

func objectMap(raw []byte) (map[string]any, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	standardized, err := hujson.Standardize(raw)
	if err != nil {
		return nil, false
	}
	var obj map[string]any
	if err := json.Unmarshal(standardized, &obj); err != nil {
		return nil, false
	}

	return obj, true
}

func valueBytes(body []byte, value *hujson.Value) []byte {
	if value == nil || value.StartOffset < 0 || value.EndOffset < value.StartOffset || value.EndOffset > len(body) {
		return nil
	}

	return body[value.StartOffset:value.EndOffset]
}

func isObject(value *hujson.Value) bool {
	return value != nil && value.Value != nil && value.Value.Kind() == '{'
}

func membersNamed(object *hujson.Value, name string) []*hujson.Value {
	obj, ok := object.Value.(*hujson.Object)
	if !ok {
		return nil
	}
	var out []*hujson.Value
	for i := range obj.Members {
		if memberName(&obj.Members[i]) == name {
			out = append(out, &obj.Members[i].Value)
		}
	}

	return out
}

func lastMember(object *hujson.Value, name string) *hujson.Value {
	all := membersNamed(object, name)
	if len(all) == 0 {
		return nil
	}

	return all[len(all)-1]
}

func memberName(member *hujson.ObjectMember) string {
	lit, ok := member.Name.Value.(hujson.Literal)
	if !ok {
		return ""
	}
	var name string
	if err := json.Unmarshal(lit, &name); err != nil {
		return ""
	}

	return name
}

func splitBOM(data []byte) (bom, body []byte) {
	if bytes.HasPrefix(data, utf8BOM) {
		return utf8BOM, data[len(utf8BOM):]
	}

	return nil, data
}

func withBOM(bom, body []byte) []byte {
	if len(bom) == 0 {
		return body
	}
	out := make([]byte, 0, len(bom)+len(body))
	out = append(out, bom...)
	out = append(out, body...)

	return out
}

// insertMember splices "name": entry into object.
//
// An empty object gets the member just before its closing brace. Otherwise
// the member goes directly after the last member's value, or after that
// member's trailing comma when it has one, so the whitespace that already
// sits before the closing brace stays where it is. When the last member
// starts on its own line the new member does too, with the same indentation,
// and the entry is indented to match; a compact object gets a compact
// member.
func insertMember(original []byte, object *hujson.Value, name string, entry []byte) ([]byte, error) {
	obj, ok := object.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New("internal: MCP config value is not a JSON object")
	}
	if len(obj.Members) == 0 {
		at, err := closeBrace(object, original)
		if err != nil {
			return nil, err
		}

		return splice(original, at, at, memberFragment(name, entry, false))
	}

	last := &obj.Members[len(obj.Members)-1]
	if last.Value.EndOffset <= 0 || last.Value.EndOffset > len(original) {
		return nil, errors.New("internal: JSON value is outside the file")
	}
	at := last.Value.EndOffset

	// A trailing comma is recorded as a non-nil AfterExtra on the last member,
	// holding whatever sits between the value and the comma. When it is
	// present the new member goes after it and needs no comma of its own.
	trailingComma := last.Value.AfterExtra != nil
	if trailingComma {
		at += len(last.Value.AfterExtra)
		if at >= len(original) || original[at] != ',' {
			return nil, errors.New("internal: trailing comma is not where the parser reported it")
		}
		at++
	}

	nl, indent := memberIndent(last.Name.BeforeExtra)
	var fragment []byte
	if !trailingComma {
		fragment = append(fragment, ',')
	}
	if nl != "" {
		fragment = append(fragment, nl...)
		fragment = append(fragment, indent...)
		entry = indentEntry(entry, indent, containerIndent(obj.AfterExtra), nl)
	}
	fragment = append(fragment, memberFragment(name, entry, false)...)

	return splice(original, at, at, fragment)
}

// memberIndent reads the line ending and indentation a member was written
// with from the whitespace and comments before its name. Both are empty
// when the member does not start on its own line.
func memberIndent(before hujson.Extra) (nl, indent string) {
	i := bytes.LastIndexByte(before, '\n')
	if i < 0 {
		return "", ""
	}
	nl = "\n"
	if i > 0 && before[i-1] == '\r' {
		nl = "\r\n"
	}
	rest := before[i+1:]
	j := len(rest)
	for j > 0 && (rest[j-1] == ' ' || rest[j-1] == '\t') {
		j--
	}

	return nl, string(rest[j:])
}

// containerIndent is the indentation of the object's closing brace, read
// from the whitespace that precedes it.
func containerIndent(after hujson.Extra) string {
	_, indent := memberIndent(after)

	return indent
}

// indentEntry re-indents a compact JSON value so that it reads as if it had
// been written at memberIndent inside an object whose closing brace sits at
// outer. The value stays compact when the indentation unit cannot be told
// from those two.
func indentEntry(entry []byte, memberIndent, outer, nl string) []byte {
	if !strings.HasPrefix(memberIndent, outer) || len(memberIndent) == len(outer) {
		return entry
	}
	unit := memberIndent[len(outer):]
	var buf bytes.Buffer
	if err := json.Indent(&buf, entry, memberIndent, unit); err != nil {
		return entry
	}
	if nl != "\n" {
		return bytes.ReplaceAll(buf.Bytes(), []byte("\n"), []byte(nl))
	}

	return buf.Bytes()
}

func closeBrace(value *hujson.Value, original []byte) (int, error) {
	if value.EndOffset <= 0 || value.EndOffset > len(original) {
		return 0, errors.New("internal: JSON value is outside the file")
	}
	at := value.EndOffset - 1
	if original[at] != '}' {
		return 0, errors.New("internal: JSON object does not end with '}'")
	}

	return at, nil
}

func memberFragment(name string, entry []byte, comma bool) []byte {
	quoted, err := json.Marshal(name)
	if err != nil {
		quoted = []byte(`"` + name + `"`)
	}
	var fragment []byte
	if comma {
		fragment = append(fragment, ',')
	}
	fragment = append(fragment, quoted...)
	fragment = append(fragment, ':', ' ')
	fragment = append(fragment, entry...)

	return fragment
}

func splice(original []byte, start, end int, insert []byte) ([]byte, error) {
	if start < 0 || end < start || end > len(original) {
		return nil, fmt.Errorf("internal: invalid edit range %d:%d", start, end)
	}
	out := make([]byte, 0, len(original)+len(insert))
	out = append(out, original[:start]...)
	out = append(out, insert...)
	out = append(out, original[end:]...)

	return out, nil
}

func validateJSON(data []byte) error {
	parsed, err := hujson.Parse(data)
	if err != nil {
		return fmt.Errorf("refusing to write JSON that does not parse: %w", err)
	}
	standardized, err := hujson.Standardize(parsed.Pack())
	if err != nil {
		return fmt.Errorf("refusing to write JSON that does not parse: %w", err)
	}
	if !json.Valid(standardized) {
		return errors.New("refusing to write JSON that does not parse")
	}

	return nil
}

// freshJSON is the document created when the config file is missing.
// It contains only the fly entry.
func freshJSON(container string, entry []byte) ([]byte, error) {
	var value any
	if err := json.Unmarshal(entry, &value); err != nil {
		return nil, err
	}
	root := map[string]any{
		container: map[string]any{mcpServerName: value},
	}
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(data, '\n'), nil
}

// jsonEqual reports whether two documents describe the same JSON value.
func jsonEqual(a, b []byte) bool {
	var left, right any
	if json.Unmarshal(a, &left) != nil || json.Unmarshal(b, &right) != nil {
		return false
	}

	return bytes.Equal(mustCompact(left), mustCompact(right))
}

func mustCompact(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		return nil
	}

	return data
}
