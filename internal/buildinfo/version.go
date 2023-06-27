package buildinfo

import (
	"errors"
	"os"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/blang/semver"
	"github.com/loadsmart/calver-go/calver"
)

type Version interface {
	String() string
	EQ(other Version) bool
	Outdated(string) (bool, error)
}

type SemverVersion struct {
	semver.Version
}

func ParseSemver(version string) (*SemverVersion, error) {
	v, err := semver.ParseTolerant(version)
	if err != nil {
		return nil, err
	}
	return &SemverVersion{v}, nil
}

func (v SemverVersion) EQ(other Version) bool {
	o, ok := other.(*SemverVersion)
	if ok {
		return v.Version.EQ(o.Version)
	} else {
		return false
	}
}

func (v *SemverVersion) Outdated(other string) (bool, error) {
	otherParsed, err := ParseSemver(other)
	if err != nil {
		return false, err
	}
	return v.LT(otherParsed.Version), nil
}

type CalverVersion struct {
	calver.Version
}

const calverFormat = "YYYY.0M.0D.MICRO"

func ParseCalver(version string) (*CalverVersion, error) {
	v, err := calver.Parse(calverFormat, version)
	if err != nil {
		return nil, err
	}
	return &CalverVersion{*v}, nil
}

func (v CalverVersion) EQ(other Version) bool {
	o, ok := other.(*CalverVersion)
	if ok {
		return v.Version.CompareTo(&o.Version) == 0
	} else {
		return false
	}
}

func (v *CalverVersion) Outdated(other string) (bool, error) {
	otherParsed, err := ParseCalver(other)
	if err != nil {
		return false, err
	}
	return v.CompareTo(&otherParsed.Version) < 0, nil
}

func ParseVersion(other string) (Version, error) {
	version, err := ParseCalver(other)
	if err != nil {
		return ParseSemver(other)
	} else {
		return version, nil
	}
}

type Versions []Version

func (v Versions) Len() int {
	return len(v)
}

func (s Versions) Less(i, j int) bool {
	v1, ok := s[i].(*CalverVersion)
	if ok {
		v2, ok := s[j].(*CalverVersion)
		if ok {
			return v1.CompareTo(&v2.Version) == -1
		} else {
			// All calver versions are > semver versions.
			return false
		}
	} else {
		v1 := s[i].(*SemverVersion)
		_, ok := s[j].(*CalverVersion)
		if ok {
			// All semver versions are < calver versions.
			return true
		} else {
			v2 := s[j].(*SemverVersion)
			return v1.Version.LT(v2.Version)
		}
	}
}

func (s Versions) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

var (
	buildDate  = "<date>"
	version    = "<version>"
	branchName = ""
)

var (
	parsedVersion   Version
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

		parsed, err := calver.NewVersion(calverFormat, int(versionNum))
		if err == nil {
			parsedVersion = &CalverVersion{Version: *parsed}
		}
	} else {
		parsedBuildDate = parsedBuildDate.UTC()

		parsed, err := ParseVersion(version)
		if err == nil {
			parsedVersion = parsed
		}
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

func ParsedVersion() Version {
	return parsedVersion
}

func BuildDate() time.Time {
	return parsedBuildDate
}
