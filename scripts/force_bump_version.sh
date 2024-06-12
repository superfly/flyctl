#!/usr/bin/env bash

set -euo pipefail

ORIGIN=${ORIGIN:-origin}

bump=${1:-patch}

dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

previous_version="$("$dir"/../scripts/version.sh -s)"

prerelversion=$("$dir"/../scripts/semver get prerel "$previous_version")
if [[ $prerelversion == "" ]]; then
  new_version=$("$dir"/../scripts/semver bump "$bump" "$previous_version")
else
  new_version=${previous_version//-$prerelversion/}
fi

new_version="v$new_version"

echo "Bumping version from v${previous_version} to ${new_version}"

git tag -m "release ${new_version}" -a "$new_version" && git push "${ORIGIN}" tag "$new_version"
