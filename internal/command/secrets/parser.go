package secrets

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

const (
	parserStateSingleline = iota
	parserStateMultiline  = iota
)

func parseSecrets(reader io.Reader) (map[string]string, error) {
	secrets := map[string]string{}
	scanner := bufio.NewScanner(reader)
	parserState := parserStateSingleline
	parsedKey := ""
	parsedVal := strings.Builder{}

	for scanner.Scan() {
		line := scanner.Text()
		switch parserState {
		case parserStateSingleline:
			// Skip comments and empty lines
			if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
				continue
			}

			key, value, ok := strings.Cut(line, "=")
			if !ok {
				return nil, fmt.Errorf("Secrets must be provided as NAME=VALUE pairs (%s is invalid)", line)
			}
			key = strings.TrimSpace(key)
			value = strings.TrimLeft(value, " ")
			l, _, ok := strings.Cut(value, "#")
			if ok && strings.Count(l, `"`)%2 == 0 {
				value = strings.TrimRight(l, " ")
			}

			if strings.HasPrefix(value, `"""`) && strings.HasSuffix(value, `"""`) && len(value) >= 6 {
				// Single-line triple-quoted string
				value = value[3 : len(value)-3]
				secrets[key] = value
			} else if strings.HasPrefix(value, `"""`) {
				// Switch to multiline
				parserState = parserStateMultiline
				parsedKey = key
				parsedVal.WriteString(strings.TrimPrefix(value, `"""`))
				parsedVal.WriteString("\n")
			} else {
				if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
					// Remove double quotes
					value = value[1 : len(value)-1]
				} else if strings.HasPrefix(value, `'`) && strings.HasSuffix(value, `'`) {
					// Remove single quotes
					value = value[1 : len(value)-1]
				}
				secrets[key] = value
			}
		case parserStateMultiline:
			if strings.HasSuffix(line, `"""`) {
				// End of multiline
				parsedVal.WriteString(strings.TrimSuffix(line, `"""`))
				secrets[parsedKey] = parsedVal.String()
				parsedVal.Reset()
				parserState = parserStateSingleline
				parsedKey = ""
			} else {
				parsedVal.WriteString(line + "\n")
			}

		}
	}

	return secrets, nil
}
