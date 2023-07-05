package buildinfo

import (
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/blang/semver"
)

var (
	buildDate  = "<date>"
	version    = "<version>"
	branchName = ""
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
		envVersionNum := os.Getenv("FLY_DEV_VERSION_NUM")
		versionNum := uint64(parsedBuildDate.Unix())

		if envVersionNum != "" {
			num, err := strconv.ParseUint(envVersionNum, 10, 64)

			if err == nil {
				versionNum = num
			}

		}

		parsedVersion = semver.Version{
			Pre: []semver.PRVersion{
				{
					VersionNum: versionNum,
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
	info, _ := debug.ReadBuildInfo()
	var rev string = "<none>"
	var dirty string = ""
	for _, v := range info.Settings {
		if v.Key == "vcs.revision" {
			rev = v.Value
		}
		if v.Key == "vcs.modified" {
			if v.Value == "true" {
				dirty = "-dirty"
			} else {
				dirty = ""
			}
		}
	}
	return rev + dirty
}

func BranchName() string {
	return branchName
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
		fmt.Printf("WARN: error parsing version number '%s': %s\n", v, err)
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
