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

			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				return nil, fmt.Errorf("Secrets must be provided as NAME=VALUE pairs (%s is invalid)", line)
			}

			if strings.HasPrefix(parts[1], `"""`) {
				// Switch to multiline
				parserState = parserStateMultiline
				parsedKey = parts[0]
				parsedVal.WriteString(strings.TrimPrefix(parts[1], `"""`))
				parsedVal.WriteString("\n")
			} else {
				value := parts[1]
				if strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
					// Remove double quotes
					value = value[1 : len(value)-1]
				}
				secrets[parts[0]] = value
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
