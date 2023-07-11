package buildinfo

import (
	"errors"
	"runtime/debug"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/loadsmart/calver-go/calver"
)

type Version interface {
	String() string
	EQ(other Version) bool
	Newer() bool
	SeverelyOutdated(other Version) bool
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

func (v *SemverVersion) Newer() bool {
	_, ok := parsedVersion.(*CalverVersion)
	if ok {
		return false
	} else {
		other := parsedVersion.(*SemverVersion)
		return v.Version.GT(other.Version)
	}
}

func (v *SemverVersion) SeverelyOutdated(other Version) bool {
	otherVer, ok := other.(*SemverVersion)
	if ok {
		if v.Major < otherVer.Major {
			return true
		}
		if v.Minor < otherVer.Minor {
			return true
		}
		if v.Patch+5 < otherVer.Patch {
			return true
		}
		return false
	} else {
		return false
	}
}

type CalverVersion struct {
	calver.Version
}

const calverFormat = "YYYY.0M.0D.MICRO"

func ParseCalver(version string) (*CalverVersion, error) {
	v, err := calver.Parse(calverFormat, version)
	if err != nil {
		// Does the version parse as calver without dashes? Needed for semverr
		// compatibility for goreleaser.
		dedashedVersion := strings.Replace(version, "-", ".", 1)
		v, err = calver.Parse(calverFormat, dedashedVersion)
		if err != nil {
			return nil, err
		}
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

func (v *CalverVersion) Newer() bool {
	other, ok := parsedVersion.(*CalverVersion)
	if ok {
		return v.Version.CompareTo(&other.Version) == 1
	} else {
		return true
	}
}

func (v *CalverVersion) SeverelyOutdated(other Version) bool {
	// todo: Figure out how to do this when we actually have outdated calver
	// versions.
	return false
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
		parsed, err := ParseVersion(version)
		if err == nil {
			parsedVersion = parsed
		} else {
			versionNum := int(parsedBuildDate.Unix())
			version, err := calver.NewVersion(calverFormat, versionNum)
			if err == nil {
				parsedVersion = &CalverVersion{Version: *version}
			}
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
