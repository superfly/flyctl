#!/bin/sh
# Based on Deno installer: Copyright 2019 the Deno authors. All rights reserved. MIT license.
# TODO(everyone): Keep this script simple and easily auditable.

set -e

os=$(uname -s)
arch=$(uname -m)
version=${1:-latest}

flyctl_uri=$(curl -s ${FLY_FORCE_TRACE:+ -H "Fly-Force-Trace: $FLY_FORCE_TRACE"} https://api.fly.io/app/flyctl_releases/$os/$arch/$version)
if [ ! "$flyctl_uri" ]; then
	echo "Error: Unable to find a flyctl release for $os/$arch/$version - see github.com/superfly/flyctl/releases for all versions" 1>&2
	exit 1
fi

flyctl_install="${FLYCTL_INSTALL:-$HOME/.fly}"

bin_dir="$flyctl_install/bin"
exe="$bin_dir/flyctl"
simexe="$bin_dir/fly"

if [ ! -d "$bin_dir" ]; then
 	mkdir -p "$bin_dir"
fi

curl -q --fail --location --progress-bar --output "$exe.tar.gz" "$flyctl_uri"
cd "$bin_dir"
tar xzf "$exe.tar.gz"
chmod +x "$exe"
rm "$exe.tar.gz"

ln -sf $exe $simexe

if [ "${1}" = "prerel" ] || [ "${1}" = "pre" ]; then
	"$exe" version -s "shell-prerel"
else
	"$exe" version -s "shell"
fi

echo "flyctl was installed successfully to $exe"
if command -v flyctl >/dev/null; then
	echo "Run 'flyctl --help' to get started"
else
	case $SHELL in
	/bin/zsh) shell_profile=".zshrc" ;;
	*) shell_profile=".bash_profile" ;;
	esac
	echo "Manually add the directory to your \$HOME/$shell_profile (or similar)"
	echo "  export FLYCTL_INSTALL=\"$flyctl_install\""
	echo "  export PATH=\"\$FLYCTL_INSTALL/bin:\$PATH\""
	echo "Run '$exe --help' to get started"
fi
