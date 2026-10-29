package setup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// mergeStatus is the outcome of a config merge that did not hit an I/O or
// parse error.
type mergeStatus int

const (
	mergeWrote mergeStatus = iota
	mergeAlready
	mergeRefused
)

type mergeResult struct {
	status mergeStatus
	data   []byte
	detail string
}

// mergeJSONBytes inserts or replaces server inside the object at container,
// leaving every other byte of src in place. A file that does not parse is
// returned as an error and data is nil, so the caller can leave it untouched.
func mergeJSONBytes(src []byte, container []string, server string, entry []byte, force bool) (mergeResult, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		src = []byte("{}")
	}
	root, err := parseJSON(src)
	if err != nil {
		return mergeResult{}, fmt.Errorf("could not parse JSON: %w", err)
	}
	if root.kind != '{' {
		return mergeResult{}, fmt.Errorf("config root is not a JSON object")
	}

	obj, idx, err := walkObjects(root, src, container)
	if err != nil {
		return mergeResult{}, err
	}
	if idx < len(container) {
		pretty := bytes.Contains(src, []byte("\n"))
		indent := "  "
		if len(obj.fields) > 0 {
			indent = keyIndent(src, obj.fields[0])
		}
		value, err := wrapMissing(container[idx:], server, entry, pretty, indent)
		if err != nil {
			return mergeResult{}, err
		}
		out, err := insertKey(src, obj, container[idx], value)
		if err != nil {
			return mergeResult{}, err
		}
		if err := validateJSON(out); err != nil {
			return mergeResult{}, err
		}

		return mergeResult{status: mergeWrote, data: out}, nil
	}

	field := findField(obj, server)
	if field == nil {
		out, err := insertKey(src, obj, server, entry)
		if err != nil {
			return mergeResult{}, err
		}
		if err := validateJSON(out); err != nil {
			return mergeResult{}, err
		}

		return mergeResult{status: mergeWrote, data: out}, nil
	}

	raw := src[field.val.start:field.val.end]
	url, isObject := jsonURL(raw)
	if isObject && url == mcpURL {
		return mergeResult{status: mergeAlready, detail: "already configured"}, nil
	}
	if !force {
		detail := fmt.Sprintf("%s entry does not point at %s", server, mcpURL)
		if url != "" {
			detail = fmt.Sprintf("%s entry points at %s", server, url)
		}

		return mergeResult{status: mergeRefused, detail: detail + "; re-run with --force to replace it"}, nil
	}

	out := splice(src, field.val.start, field.val.end, entry)
	if err := validateJSON(out); err != nil {
		return mergeResult{}, err
	}

	return mergeResult{status: mergeWrote, data: out}, nil
}

func validateJSON(out []byte) error {
	if !json.Valid(out) {
		return fmt.Errorf("internal error: merge produced invalid JSON")
	}

	return nil
}

// jsonURL returns the endpoint an MCP entry points at. serverUrl is
// Antigravity's name for the same thing; url is what the other clients use.
// isObject is false when the value is not a JSON object.
func jsonURL(raw []byte) (url string, isObject bool) {
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", false
	}
	for _, key := range []string{"url", "serverUrl"} {
		if s, ok := v[key].(string); ok && s != "" {
			return s, true
		}
	}

	return "", true
}

// wrapMissing builds the JSON value for the first missing path element. The
// value already contains the fly server, so the caller only inserts one key.
func wrapMissing(missing []string, server string, entry []byte, pretty bool, indent string) ([]byte, error) {
	val, err := jsonObject(server, entry, pretty, indent+strings.Repeat("  ", len(missing)))
	if err != nil {
		return nil, err
	}
	for i := len(missing) - 1; i >= 1; i-- {
		val, err = jsonObject(missing[i], val, pretty, indent+strings.Repeat("  ", i))
		if err != nil {
			return nil, err
		}
	}

	return val, nil
}

func jsonObject(key string, value []byte, pretty bool, indent string) ([]byte, error) {
	quoted, err := json.Marshal(key)
	if err != nil {
		return nil, fmt.Errorf("could not encode JSON key: %w", err)
	}
	if !pretty {
		out := make([]byte, 0, len(quoted)+len(value)+3)
		out = append(out, '{')
		out = append(out, quoted...)
		out = append(out, ':')
		out = append(out, value...)
		out = append(out, '}')

		return out, nil
	}
	var b bytes.Buffer
	b.WriteString("{\n")
	b.WriteString(indent)
	b.WriteString("  ")
	b.Write(quoted)
	b.WriteString(": ")
	b.Write(value)
	b.WriteByte('\n')
	b.WriteString(indent)
	b.WriteByte('}')

	return b.Bytes(), nil
}

func insertKey(src []byte, obj *jnode, key string, value []byte) ([]byte, error) {
	quoted, err := json.Marshal(key)
	if err != nil {
		return nil, fmt.Errorf("could not encode JSON key: %w", err)
	}
	pretty := bytes.Contains(src[obj.start:obj.end], []byte("\n"))
	if len(obj.fields) == 0 {
		var body []byte
		if pretty {
			body, err = jsonObject(key, value, true, indentAt(src, obj.start))
		} else {
			body, err = jsonObject(key, value, false, "")
		}
		if err != nil {
			return nil, err
		}

		return splice(src, obj.start, obj.end, body), nil
	}
	last := obj.fields[len(obj.fields)-1]
	var ins []byte
	if pretty {
		indent := keyIndent(src, obj.fields[0])
		ins = append(ins, ',', '\n')
		ins = append(ins, indent...)
		ins = append(ins, quoted...)
		ins = append(ins, ':', ' ')
		ins = append(ins, value...)
	} else {
		ins = append(ins, ',')
		ins = append(ins, quoted...)
		ins = append(ins, ':')
		ins = append(ins, value...)
	}

	return splice(src, last.val.end, last.val.end, ins), nil
}

func indentAt(src []byte, pos int) string {
	line := pos
	for line > 0 && src[line-1] != '\n' {
		line--
	}
	var indent []byte
	for i := line; i < pos; i++ {
		if src[i] != ' ' && src[i] != '\t' {
			break
		}
		indent = append(indent, src[i])
	}

	return string(indent)
}

func keyIndent(src []byte, field jfield) string {
	return indentAt(src, field.keyStart)
}

func splice(src []byte, start, end int, insert []byte) []byte {
	out := make([]byte, 0, len(src)-(end-start)+len(insert))
	out = append(out, src[:start]...)
	out = append(out, insert...)
	out = append(out, src[end:]...)

	return out
}

// walkObjects walks container. When the full path exists, obj is that object
// and idx is len(container). Otherwise obj is the deepest existing object and
// idx is the first missing element. src is only used for error text.
func walkObjects(root *jnode, src []byte, container []string) (obj *jnode, idx int, err error) {
	obj = root
	for idx = 0; idx < len(container); idx++ {
		if obj.kind != '{' {
			return nil, idx, fmt.Errorf("%s is not a JSON object", strings.Join(container[:idx], "."))
		}
		field := findField(obj, container[idx])
		if field == nil {
			return obj, idx, nil
		}
		if field.val.kind != '{' {
			name := strings.Join(container[:idx+1], ".")
			return nil, idx, fmt.Errorf("%s is not a JSON object (%s)", name, snippet(src[field.val.start:field.val.end]))
		}
		obj = field.val
	}

	return obj, idx, nil
}

func snippet(b []byte) string {
	s := string(b)
	if len(s) > 40 {
		s = s[:40] + "..."
	}

	return s
}

type jnode struct {
	start, end int
	kind       byte
	fields     []jfield
}

type jfield struct {
	key      string
	keyStart int
	val      *jnode
}

type jparser struct {
	src []byte
	i   int
}

func parseJSON(src []byte) (*jnode, error) {
	p := &jparser{src: src}
	p.skipWS()
	if p.i >= len(p.src) {
		return nil, fmt.Errorf("empty document")
	}
	node, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	p.skipWS()
	if p.i != len(p.src) {
		return nil, fmt.Errorf("trailing data after JSON value")
	}

	return node, nil
}

func (p *jparser) skipWS() {
	for p.i < len(p.src) {
		switch p.src[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *jparser) parseValue() (*jnode, error) {
	p.skipWS()
	if p.i >= len(p.src) {
		return nil, fmt.Errorf("unexpected end of JSON")
	}
	start := p.i
	switch p.src[p.i] {
	case '{':
		return p.parseObject()
	case '[':
		return p.parseArray()
	case '"':
		end, err := p.parseString()
		if err != nil {
			return nil, err
		}

		return &jnode{start: start, end: end, kind: '"'}, nil
	case 't':
		if err := p.consume("true"); err != nil {
			return nil, err
		}

		return &jnode{start: start, end: p.i, kind: 'o'}, nil
	case 'f':
		if err := p.consume("false"); err != nil {
			return nil, err
		}

		return &jnode{start: start, end: p.i, kind: 'o'}, nil
	case 'n':
		if err := p.consume("null"); err != nil {
			return nil, err
		}

		return &jnode{start: start, end: p.i, kind: 'o'}, nil
	default:
		if err := p.parseNumber(); err != nil {
			return nil, err
		}

		return &jnode{start: start, end: p.i, kind: 'o'}, nil
	}
}

func (p *jparser) parseObject() (*jnode, error) {
	start := p.i
	p.i++
	var fields []jfield
	for {
		p.skipWS()
		if p.i >= len(p.src) {
			return nil, fmt.Errorf("unterminated object")
		}
		if p.src[p.i] == '}' {
			p.i++

			return &jnode{start: start, end: p.i, kind: '{', fields: fields}, nil
		}
		if len(fields) > 0 {
			if p.src[p.i] != ',' {
				return nil, fmt.Errorf("expected comma between object fields")
			}
			p.i++
			p.skipWS()
		}
		keyStart := p.i
		keyEnd, err := p.parseString()
		if err != nil {
			return nil, err
		}
		var key string
		if err := json.Unmarshal(p.src[keyStart:keyEnd], &key); err != nil {
			return nil, err
		}
		p.skipWS()
		if p.i >= len(p.src) || p.src[p.i] != ':' {
			return nil, fmt.Errorf("expected colon after object key")
		}
		p.i++
		val, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		fields = append(fields, jfield{key: key, keyStart: keyStart, val: val})
	}
}

func (p *jparser) parseArray() (*jnode, error) {
	start := p.i
	p.i++
	first := true
	for {
		p.skipWS()
		if p.i >= len(p.src) {
			return nil, fmt.Errorf("unterminated array")
		}
		if p.src[p.i] == ']' {
			p.i++

			return &jnode{start: start, end: p.i, kind: '['}, nil
		}
		if !first {
			if p.src[p.i] != ',' {
				return nil, fmt.Errorf("expected comma between array elements")
			}
			p.i++
		}
		first = false
		if _, err := p.parseValue(); err != nil {
			return nil, err
		}
	}
}

func (p *jparser) parseString() (int, error) {
	if p.i >= len(p.src) || p.src[p.i] != '"' {
		return 0, fmt.Errorf("expected string")
	}
	p.i++
	for p.i < len(p.src) {
		c := p.src[p.i]
		if c == '\\' {
			if p.i+1 >= len(p.src) {
				return 0, fmt.Errorf("unterminated escape in string")
			}
			p.i += 2

			continue
		}
		if c == '"' {
			p.i++

			return p.i, nil
		}
		if c < 0x20 {
			return 0, fmt.Errorf("invalid control character in string")
		}
		p.i++
	}

	return 0, fmt.Errorf("unterminated string")
}

func (p *jparser) consume(lit string) error {
	if p.i+len(lit) > len(p.src) || string(p.src[p.i:p.i+len(lit)]) != lit {
		return fmt.Errorf("expected %s", lit)
	}
	p.i += len(lit)

	return nil
}

func (p *jparser) parseNumber() error {
	start := p.i
	if p.src[p.i] == '-' {
		p.i++
	}
	if p.i >= len(p.src) || !isDigit(p.src[p.i]) {
		return fmt.Errorf("expected number")
	}
	if p.src[p.i] == '0' {
		p.i++
	} else {
		for p.i < len(p.src) && isDigit(p.src[p.i]) {
			p.i++
		}
	}
	if p.i < len(p.src) && p.src[p.i] == '.' {
		p.i++
		if p.i >= len(p.src) || !isDigit(p.src[p.i]) {
			return fmt.Errorf("expected digit after decimal point")
		}
		for p.i < len(p.src) && isDigit(p.src[p.i]) {
			p.i++
		}
	}
	if p.i < len(p.src) && (p.src[p.i] == 'e' || p.src[p.i] == 'E') {
		p.i++
		if p.i < len(p.src) && (p.src[p.i] == '+' || p.src[p.i] == '-') {
			p.i++
		}
		if p.i >= len(p.src) || !isDigit(p.src[p.i]) {
			return fmt.Errorf("expected digit in exponent")
		}
		for p.i < len(p.src) && isDigit(p.src[p.i]) {
			p.i++
		}
	}
	if p.i == start {
		return fmt.Errorf("expected number")
	}

	return nil
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// findField returns the last field with key, matching encoding/json, which
// keeps the last duplicate.
func findField(n *jnode, key string) *jfield {
	var found *jfield
	for i := range n.fields {
		if n.fields[i].key == key {
			found = &n.fields[i]
		}
	}

	return found
}
