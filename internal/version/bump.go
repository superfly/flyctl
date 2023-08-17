package version

func IncrementMajor(v Version) Version {
	v.Major++
	v.Minor = 0
	v.Patch = 0
	v.Build = 0
	return v
}
