package cmdutil

import (
	"fmt"
	"strings"
)

// ParseKVStringsToMap converts a slice of NAME=VALUE strings into a map[string]string
func ParseKVStringsToMap(args []string) (map[string]string, error) {
	out := make(map[string]string, len(args))

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("'%s': must be in the format NAME=VALUE", arg)
		}
		out[parts[0]] = parts[1]
	}

	return out, nil
}
