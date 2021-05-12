#!/usr/bin/env bash

current_tag=$(git tag --points-at HEAD --sort -v:refname | head -n1)
if [ -z "$current_tag" ]; then
  current_tag=$(git describe --tags --abbrev=0)
fi

>&2 echo "current tag: $current_tag"

# if the current tag is a prerelease, get the previous tag, otherwise get the previous non-prerelease tag
if [[ $current_tag =~ pre ]]; then
  previous_tag=$(git describe --match "v[0-9]*" --abbrev=0 HEAD^)
else
  previous_tag=$(git describe --match "v[0-9]*" --exclude "*-pre-*" --abbrev=0 HEAD^)
fi

>&2 echo "previous tag: $previous_tag"

# only include go files in the changelog
git log --oneline --no-merges --no-decorate $previous_tag..HEAD -- '*.go' '**/*.go' 
