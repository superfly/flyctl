// Package env implements environment related functionality.
package env

import (
	"os"
	"strings"
)

// GetDef retrieves the value of the environment variable named by the key. In
// case the variable is not present, GetOr returns def.
func GetDef(key, def string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}

	return def
}

// Get is shorthand for GetDef(key, "").
func Get(key string) string {
	return GetDef(key, "")
}

// IsTruthy reports that the value of environment variable named by the key is
// evaluates to true. IsTruthy always reports false for unset variables.
func IsTruthy(key string) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return false
	}

	switch strings.ToLower(v) {
	default:
		return false
	case "1", "ok", "t", "true":
		return true
	}
}

// IsSet reports whether the environment variable named by the key is set.
func IsSet(key string) (is bool) {
	_, is = os.LookupEnv(key)

	return
}

// https://github.com/watson/ci-info/blob/c4f1553f254c78babef5c200c48569ede313b718/index.js
var ciKeys = []string{
	// Travis CI, CircleCI, Cirrus CI,
	// Gitlab CI, Appveyor, CodeShip, dsari
	"CI",

	// Travis CI, Cirrus CI
	"CONTINUOUS_INTEGRATION",

	// Jenkins, TeamCity
	"BUILD_NUMBER",

	// TaskCluster, dsari
	"RUN_ID",
}

// IsCI reports whether the environment is a CI one.
func IsCI() bool {
	for _, key := range ciKeys {
		if IsSet(key) {
			return true
		}
	}

	return false
}
