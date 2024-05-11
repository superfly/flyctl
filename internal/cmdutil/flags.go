package cmdutil

import (
	"fmt"
	"regexp"
	"strings"
)

var envNameRegex = regexp.MustCompile(`^[a-zA-Z_][-a-zA-Z0-9_]*$`)

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

// ParseENVStringtoMap is just like ParseKVStringsToMap, but it will map
// names without values to FLY_ENV=name.
func ParseENVStringsToMap(args []string) (map[string]string, error) {
	out := make(map[string]string, len(args))

	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) == 1 && envNameRegex.MatchString(parts[0]) {
			out["FLY_ENV"] = parts[0]
		} else if len(parts) != 2 {
			return nil, fmt.Errorf("'%s': must be in the format NAME=VALUE", arg)
		}
		out[parts[0]] = parts[1]
	}

	return out, nil
}
