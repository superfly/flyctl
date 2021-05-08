# Building flyctl

## Notes on versioning

There are two version types, in line with semver. Releases with major.minor.patch versions and prereleases with major.minor.patch-pre-incnumber versions.

The process has been for a release 

scripts/bump-version.sh [increment field]

the version field named (major/minor/patch (the default)) would be incremented and the act of the new tag being pushed trigger a goreleaser instance to publish.

With prerel support, a new optional parameter, prerel can be used along with the version field. It's action depends on the current version number.

v0.0.100 -> where the number is a release version, the (default) patch is incremented, then a -pre-1 prerelease tag is added

v0.0.101-pre-1 -> if the prerel parameter is present, then the tag is incremented to -pre-2 and so on
                    if the prerel parameter is not present, then the prerel tag is removed and the new version is v0.0.101 -> a 
                    release


## Install logic

If the install.sh script is called as normal, then the latest release version is downloaded
If the install.sh script is called with prerel as a parameter, then the latest prerelease version is installed*

* if there is no current prerelease version available, the latest release version is installed. This allowes prerel users to be gently ushered back to the mainline releases.

## Update logic

If the version running is a release version, it will only notify of a new release release. Pre-releases will be ignored.
If the version running is a prerelease version, it will notify on any new prerelease *or* release. If a release has happened and a subsequent prerelease has followed, the upgrade prompt will suggest the latest prerelease.

