package version

import (
	"fmt"
	"strconv"
	"strings"
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
	// old v0.1.xxx format always bumps version on every release, so no track or build on stable
	if isOldStringFormat(v) {
		if (v.Track == "stable" || v.Track == "") && v.Build <= 1 {
			return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
		}
		return fmt.Sprintf("%d.%d.%d-%s-%d", v.Major, v.Minor, v.Patch, v.Track, v.Build)
	}

	if isCalVer(v) {
		if v.Track != "" && v.Build != 0 {
			return fmt.Sprintf("%d.%d.%d-%s.%d", v.Major, v.Minor, v.Patch, v.Track, v.Build)
		}

		if v.Track != "" && v.Build == 0 {
			return fmt.Sprintf("%d.%d.%d-%s", v.Major, v.Minor, v.Patch, v.Track)
		}

		return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
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

func isOldStringFormat(v Version) bool {
	return v.Major == 0 && v.Minor == 1
}

func (v Version) Eq(other Version) bool {
	return Compare(v, other) == 0
}

func (v Version) Newer(other Version) bool {
	return Compare(v, other) == 1
}

func (v Version) Older(other Version) bool {
	return Compare(v, other) == -1
}

func Compare(a Version, b Version) int {
	if a.Major != b.Major {
		if a.Major > b.Major {
			return 1
		} else {
			return -1
		}
	}

	if a.Minor != b.Minor {
		if a.Minor > b.Minor {
			return 1
		} else {
			return -1
		}
	}

	if a.Patch != b.Patch {
		if a.Patch > b.Patch {
			return 1
		} else {
			return -1
		}
	}

	if a.Track != b.Track {
		// in semver, if one version has a prerel and the other doesn't, the one without is newer
		if a.Track == "" && b.Track != "" {
			return 1
		} else if b.Track == "" && a.Track != "" {
			return -1
		} else {
			if a.Track > b.Track {
				return 1
			} else {
				return -1
			}
		}
	}

	if a.Build != b.Build {
		if a.Build > b.Build {
			return 1
		} else {
			return -1
		}
	}

	return 0
}

func Parse(version string) (Version, error) {
	version = strings.TrimPrefix(version, "v")

	out := Version{}

	parts := strings.SplitN(version, "-", 2)
	versionStr := parts[0]
	suffixStr := ""
	if len(parts) == 2 {
		suffixStr = parts[1]
	}

	parts = strings.SplitN(versionStr, ".", 3)

	if len(parts) != 3 {
		return Version{}, &InvalidVersionError{version}
	}

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
		parts = strings.SplitN(suffixStr, ".", 2)
		// special case for old v0.1.xxx format where pre was dash separated (eg pre-123) not dot separated
		if len(parts) == 1 && strings.HasPrefix(suffixStr, "pre-") {
			parts = strings.SplitN(suffixStr, "-", 2)
		}

		out.Track = parts[0]

		if len(parts) == 2 {
			if x, err := strconv.Atoi(parts[1]); err != nil {
				return Version{}, &InvalidVersionError{version}
			} else {
				out.Build = x
			}
		} else {
			out.Build = 1
		}
	} else {
		out.Build = 1
		out.Track = "stable"
	}

	return out, nil
}
