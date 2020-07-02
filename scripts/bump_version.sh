#!/usr/bin/env bash

set -euo pipefail

ORIGIN=${ORIGIN:-origin}

bump=${1:-patch}

prerel=${2:-none}

if [[ $bump == "prerel" ]]; then
  bump="patch"
  prerel="prerel"
fi

if [[ $(git status --porcelain) != "" ]]; then
  echo "Error: repo is dirty. Run git status, clean repo and try again."
  exit 1
elif [[ $(git status --porcelain -b | grep -e "ahead" -e "behind") != "" ]]; then
  echo "Error: repo has unpushed commits. Push commits to remote and try again."
  exit 1
fi  

dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

previous_version="$("$dir"/../scripts/version.sh -s)"

if [[ $prerel == "prerel" ]]; then
  prerelversion=$("$dir"/../scripts/semver get prerel "$previous_version")
  if [[ $prerelversion == "" ]]; then
    new_version=$("$dir"/../scripts/semver bump "$bump" "$previous_version")
    new_version=$("$dir"/../scripts/semver bump prerel beta-1 "$new_version")
  else
    prerel=beta-$((${prerelversion#beta-} + 1))
    new_version=$("$dir"/../scripts/semver bump prerel "$prerel" "$previous_version")
  fi
else
  prerelversion=$("$dir"/../scripts/semver get prerel "$previous_version")
  if [[ $prerelversion == "" ]]; then
    new_version=$("$dir"/../scripts/semver bump "$bump" "$previous_version")
  else
    new_version=${previous_version//-$prerelversion/}
  fi
fi

new_version="v$new_version"

echo "Bumping version from v${previous_version} to ${new_version}"

read -p "Are you sure? " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]
then
  git tag -m "release ${new_version}" -a "$new_version" && git push "${ORIGIN}" tag "$new_version"
  echo "done"
fi

