package buildinfo

var environment = "staging"

func Environment() string {
	return environment
}

func IsDev() bool {
	return environment == "development"
}

func IsRelease() bool {
	return !IsDev()
}
