package cmdutil

import (
	"fmt"
	"strings"
)

// ParseKVStringsToMap converts a slice of NAME=VALUE strings into a map[string]string
func ParseKVStringsToMap(args []string) (map[string]string, error) {
	out := make(map[string]string, len(args))

	for i, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("argument %d must be in the format NAME=VALUE", i+1)
		}
		out[parts[0]] = parts[1]
	}

	return out, nil
}
