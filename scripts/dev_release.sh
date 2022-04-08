#!/usr/bin/env bash

set -euo pipefail

ORIGIN=${ORIGIN:-origin}

dir="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
previous_version="$("$dir"/../scripts/version.sh -s)"

if [[ $(git status --porcelain) != "" ]]; then
  echo "Error: repo is dirty. Run git status, clean repo and try again."
  exit 1
elif [[ $(git status --porcelain -b | grep -e "ahead" -e "behind") != "" ]]; then
  echo "Error: repo has unpushed commits. Push commits to remote and try again."
  exit 1
fi

revision=$(git rev-parse --short HEAD)
branch=$(git rev-parse --abbrev-ref HEAD)
version="v${previous_version}-dev-${branch/\//-}-${revision}"
echo "Publishing development release: $version"

read -p "Are you sure? " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]
then
  git tag -m "release ${version}" -a "$version" && git push "${ORIGIN}" tag "$version"
  echo "done"
fi
