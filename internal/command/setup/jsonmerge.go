package setup

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/tailscale/hujson"
)

// mergeJSONBytes inserts the fly server into a JSON or JSONC document.
//
// Existing bytes are preserved apart from the inserted member. The edit is a
// splice at an offset reported by the parser, not a re-serialize, so comments,
// key order and unrelated servers stay byte-for-byte. A document that does not
// parse is returned untouched via a non-nil error; callers must not write it.
//
// out is nil when the file should not change (already configured, or a
// conflicting entry and force is false).
func mergeJSONBytes(original []byte, client mcpClient, force bool) (out []byte, status string, err error) {
	entry, err := client.entryJSON()
	if err != nil {
		return nil, "", err
	}

	root, err := hujson.Parse(original)
	if err != nil {
		return nil, "", fmt.Errorf("not valid JSON: %w", err)
	}
	if root.Value == nil || root.Value.Kind() != '{' {
		return nil, "", errors.New("not valid JSON: the file is not a JSON object")
	}

	containerPtr := "/" + client.container
	container := root.Find(containerPtr)
	if container != nil && (container.Value == nil || container.Value.Kind() != '{') {
		return nil, "", fmt.Errorf("%s is not a JSON object", client.container)
	}

	if container != nil {
		if fly := root.Find(containerPtr + "/" + mcpServerName); fly != nil {
			points, ok := entryPointsAt(fly, client.urlKeys)
			switch {
			case ok && points:
				return nil, statusAlready, nil
			case !force:
				return nil, statusConflict, nil
			default:
				replaced, err := replaceValue(original, fly, entry)
				if err != nil {
					return nil, "", err
				}
				if err := validateJSON(replaced); err != nil {
					return nil, "", err
				}

				return replaced, statusConfigured, nil
			}
		}
	}

	var edited []byte
	if container == nil {
		wrapped := append(append([]byte(`{"`+mcpServerName+`":`), entry...), '}')
		edited, err = insertMember(original, &root, client.container, wrapped)
	} else {
		edited, err = insertMember(original, container, mcpServerName, entry)
	}
	if err != nil {
		return nil, "", err
	}
	if err := validateJSON(edited); err != nil {
		return nil, "", err
	}

	return edited, statusConfigured, nil
}

// entryPointsAt reports whether value is a JSON object whose URL field is the
// hosted server. ok is false when value is not an object.
func entryPointsAt(value *hujson.Value, keys []string) (points, ok bool) {
	if value == nil || value.Value == nil || value.Value.Kind() != '{' {
		return false, false
	}
	standardized, err := hujson.Standardize(value.Pack())
	if err != nil {
		return false, true
	}
	var obj map[string]any
	if err := json.Unmarshal(standardized, &obj); err != nil {
		return false, true
	}
	for _, key := range keys {
		if url, isString := obj[key].(string); isString && url == hostedMCPURL {
			return true, true
		}
	}

	return false, true
}

// insertMember splices "name": entry into object, before its closing brace.
func insertMember(original []byte, object *hujson.Value, name string, entry []byte) ([]byte, error) {
	obj, ok := object.Value.(*hujson.Object)
	if !ok {
		return nil, errors.New("internal: MCP config value is not a JSON object")
	}
	at, err := closeBrace(object, original)
	if err != nil {
		return nil, err
	}

	// A trailing comma is recorded on the last member. When it is absent the
	// new member needs its own comma; when it is present the comma is already
	// in the bytes we leave alone.
	needComma := len(obj.Members) > 0 && obj.Members[len(obj.Members)-1].Value.AfterExtra == nil
	fragment := memberFragment(name, entry, needComma)

	return splice(original, at, at, fragment)
}

// replaceValue overwrites one JSON value and leaves every surrounding byte.
func replaceValue(original []byte, value *hujson.Value, entry []byte) ([]byte, error) {
	if value.StartOffset < 0 || value.EndOffset < value.StartOffset || value.EndOffset > len(original) {
		return nil, errors.New("internal: JSON value is outside the file")
	}

	return splice(original, value.StartOffset, value.EndOffset, entry)
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
