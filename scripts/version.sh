ORIGIN=${ORIGIN:-origin}

version=$(git fetch --tags "${ORIGIN}" &>/dev/null |  git -c "versionsort.prereleasesuffix=-pre" tag -l --sort=version:refname | tail -n1 | cut -c 2-)

echo "$version"