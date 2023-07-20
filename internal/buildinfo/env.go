package buildinfo

import "strings"

var environment = "development"

func Environment() string {
	return environment
}

func IsDev() bool {
	return strings.Contains(version, "-snapshot.") || environment == "development"
}

func IsRelease() bool {
	return !IsDev()
}
