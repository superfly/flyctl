ORIGIN=${ORIGIN:-origin}

version=$(git fetch --tags "${ORIGIN}" &>/dev/null | git tag -l | sort --version-sort | tail -n1 | cut -c 2-)

echo "$version"