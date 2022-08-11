package buildinfo

import (
	"errors"
	"time"

	"github.com/blang/semver"
	"github.com/superfly/flyctl/terminal"
)

var (
	buildDate = "<date>"
	version   = "<version>"
	commit    = "<commit>"
)

var (
	parsedVersion   semver.Version
	parsedBuildDate time.Time
)

func init() {
	loadMeta()
}

func loadMeta() {
	var err error

	parsedBuildDate, err = time.Parse(time.RFC3339, buildDate)
	var parseErr *time.ParseError
	if errors.As(err, &parseErr) && IsDev() {
		parsedBuildDate = time.Now()
	} else if err != nil {
		panic(err)
	}

	if IsDev() {
		parsedVersion = semver.Version{
			Pre: []semver.PRVersion{
				{
					VersionNum: uint64(parsedBuildDate.Unix()),
					IsNum:      true,
				},
			},
			Build: []string{"dev"},
		}
	} else {
		parsedBuildDate = parsedBuildDate.UTC()

		parsedVersion = semver.MustParse(version)
	}
}

func Commit() string {
	return commit
}

func Version() semver.Version {
	return parsedVersion
}

func BuildDate() time.Time {
	return parsedBuildDate
}

func parseVesion(v string) semver.Version {
	parsedV, err := semver.ParseTolerant(v)
	if err != nil {
		terminal.Warnf("error parsing version number '%s': %s\n", v, err)
		return semver.Version{}
	}
	return parsedV
}

func IsVersionSame(otherVerison string) bool {
	return parsedVersion.EQ(parseVesion(otherVerison))
}

func IsVersionOlder(otherVerison string) bool {
	return parsedVersion.LT(parseVesion(otherVerison))
}

func IsVersionNewer(otherVerison string) bool {
	return parsedVersion.GT(parseVesion(otherVerison))
}
