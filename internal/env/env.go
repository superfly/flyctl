// Package env implements environment related functionality.
package env

import "os"

// IsCI reports whether the environment is a CI one.
//
// Based on https://github.com/watson/ci-info/blob/HEAD/index.js
func IsCI() bool {
	return os.Getenv("CI") != "" || // GitHub Actions, Travis CI, CircleCI, Cirrus CI, GitLab CI, AppVeyor, CodeShip, dsari
		os.Getenv("BUILD_NUMBER") != "" || // Jenkins, TeamCity
		os.Getenv("RUN_ID") != "" // TaskCluster, dsari
}
