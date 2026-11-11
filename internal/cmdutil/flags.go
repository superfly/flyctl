package cmdutil

import (
	"fmt"
	"strings"
)

// ParseKVStringsToMap converts a slice of NAME=VALUE positional arguments into
// a map[string]string. Errors identify the argument by position and never echo
// its contents, since values may be secrets.
func ParseKVStringsToMap(args []string) (map[string]string, error) {
	return parseKVStrings(args, "argument %d")
}

// ParseKVFlagToMap converts the NAME=VALUE values of a repeatable flag into a
// map[string]string. Errors name the flag and which of its values is
// malformed, and never echo the value, since it may be a secret.
func ParseKVFlagToMap(flagName string, values []string) (map[string]string, error) {
	return parseKVStrings(values, "--"+flagName+" value %d")
}

func parseKVStrings(args []string, position string) (map[string]string, error) {
	out := make(map[string]string, len(args))

	for i, arg := range args {
		name, value, ok := strings.Cut(arg, "=")
		if !ok {
			return nil, fmt.Errorf(position+" must be in the format NAME=VALUE", i+1)
		}
		out[name] = value
	}

	return out, nil
}
