package version

import (
	"cmp"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type InvalidVersionError struct {
	val string
}

func (e *InvalidVersionError) Error() string {
	return fmt.Sprintf("invalid version: %s", e.val)
}

type Version struct {
	Major int
	Minor int
	Patch int
	Build int
	Track string
}

func (v Version) String() string {
	// TODO[md]: remove this when we're done with the semver to calver migration
	// handle old v0.[1-2].XXX[-pre-X] format first
	if !isCalVer(v) && !isDev(v) {
		// version is bumped on every release -- no track or build on stable
		if v.Track == "stable" || v.Track == "" {
			return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
		}
		if v.Track == "pre" || v.Track == "beta" {
			return fmt.Sprintf("%d.%d.%d-%s-%d", v.Major, v.Minor, v.Patch, v.Track, v.Build)
		}
		return fmt.Sprintf("%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Track)
	}

	if v.Track != "" && v.Build != 0 {
		return fmt.Sprintf("%d.%d.%d-%s.%d", v.Major, v.Minor, v.Patch, v.Track, v.Build)
	}

	if v.Track != "" && v.Build == 0 {
		return fmt.Sprintf("%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Track)
	}

	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// flag to indicate which side of the semver to calver migration we're on. drop when we're done
func isCalVer(v Version) bool {
	return v.Major > 2022
}

func isDev(v Version) bool {
	return v.Major == 0 && v.Minor == 0 && v.Patch == 0 && v.Track == "dev"
}

func (v Version) Equal(other Version) bool {
	return Compare(v, other) == 0
}

func (v Version) Newer(other Version) bool {
	return Compare(v, other) == 1
}

func (v Version) Older(other Version) bool {
	return Compare(v, other) == -1
}

// Compare returns
//
//	-1 if x is less than y,
//	 0 if x equals y,
//	+1 if x is greater than y.
//
// Versions with tracks are considered less than versions without tracks, as
// per semver spec. If both versions have tracks, they are compared as strings.
// A track of "stable" is greater than any other track.
func Compare(x Version, y Version) int {
	if ret := cmp.Compare(x.Major, y.Major); ret != 0 {
		return ret
	}

	if ret := cmp.Compare(x.Minor, y.Minor); ret != 0 {
		return ret
	}

	if ret := cmp.Compare(x.Patch, y.Patch); ret != 0 {
		return ret
	}

	// in semver, if one version has a prerel and the other doesn't, the one without is newer
	if x.Track == "" && y.Track != "" {
		return 1
	} else if y.Track == "" && x.Track != "" {
		return -1
	} else {
		if ret := strings.Compare(x.Track, y.Track); ret != 0 {
			return ret
		}
	}

	if ret := cmp.Compare(x.Build, y.Build); ret != 0 {
		return ret
	}

	return 0
}

func (v Version) dateFromVersion() time.Time {
	return time.Date(v.Major, time.Month(v.Minor), v.Patch, 0, 0, 0, 0, time.UTC)
}

func (v Version) SignificantlyBehind(latest Version) bool {
	// both versions are calver, use date comparison. >28 days is old
	if isCalVer(latest) && isCalVer(v) {
		latestDate := latest.dateFromVersion()
		currentDate := v.dateFromVersion()
		fmt.Println("date diff", latestDate, currentDate, latestDate.Sub(currentDate))
		return latestDate.Sub(currentDate) >= 28*24*time.Hour
	}

	// latest is calver, current is not. consider out of date if latest is >30 days old
	if isCalVer(latest) && !isCalVer(v) {
		latestDate := latest.dateFromVersion()
		return time.Until(latestDate) >= 28*24*time.Hour
	}

	// both are old format, consider 5 patches old
	fmt.Println("a", latest.Patch, "b", v.Patch, latest.Patch-v.Patch)
	if latest.Major > v.Major {
		return true
	}
	if latest.Minor > v.Minor {
		return true
	}
	if latest.Patch > v.Patch+5 {
		return true
	}
	return false
}

func Parse(version string) (Version, error) {
	version = strings.TrimPrefix(version, "v")

	out := Version{}

	parts := strings.SplitN(version, "-", 2)
	// versionStr contains "MAJOR.MINOR.PATCH"
	versionStr := parts[0]
	suffixStr := ""
	// if parts has a length of 2, suffixStr contains "TRACK.BUILD" or "TRACK-BUILD" (latter is old format)
	if len(parts) == 2 {
		suffixStr = parts[1]
	}

	// split versionStr into "MAJOR", "MINOR", "PATCH"
	parts = strings.SplitN(versionStr, ".", 3)

	// version must have 3 parts
	if len(parts) != 3 {
		return Version{}, &InvalidVersionError{version}
	}

	// if any part is not an integer, return an error
	if x, err := strconv.Atoi(parts[0]); err != nil {
		return Version{}, &InvalidVersionError{version}
	} else {
		out.Major = x
	}
	if x, err := strconv.Atoi(parts[1]); err != nil {
		return Version{}, &InvalidVersionError{version}
	} else {
		out.Minor = x
	}
	if x, err := strconv.Atoi(parts[2]); err != nil {
		return Version{}, &InvalidVersionError{version}
	} else {
		out.Patch = x
	}

	if suffixStr != "" {
		// handle old v0.1.xxx format first, which separated track and build with a dash
		// old tracks began with either "pre-", or "beta-"
		if !isCalVer(out) && (strings.HasPrefix(suffixStr, "pre-") || strings.HasPrefix(suffixStr, "beta-")) {
			parts = strings.SplitN(suffixStr, "-", 2)
		} else {
			// handle new calver format, which separates track and build with a dot
			parts = strings.SplitN(suffixStr, ".", 2)
		}

		out.Track = parts[0]

		if len(parts) == 2 {
			if x, err := strconv.Atoi(parts[1]); err != nil {
				return Version{}, &InvalidVersionError{version}
			} else {
				out.Build = x
			}
		} else {
			out.Build = 0
		}
	} else {
		out.Build = 0
		out.Track = ""
	}

	return out, nil
}
