package buildinfo

var environment = "development"

func Environment() string {
	return environment
}

func IsDev() bool {
	// TODO[md]: handle -snapshot in new version number. is this only for testflight?
	// return strings.Contains(version, "-snapshot.") || environment == "development"
	return environment == "development"
}

func IsRelease() bool {
	return !IsDev()
}
