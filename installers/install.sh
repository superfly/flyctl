#!/bin/sh
# Based on Deno installer: Copyright 2019 the Deno authors. All rights reserved. MIT license.
# TODO(everyone): Keep this script simple and easily auditable.

set -e

if [ "$(uname -m)" != "x86_64" ]; then
	echo "Error: Unsupported architecture $(uname -m). Only x64 binaries are available." 1>&2
	exit 1
fi

# We are using tar and its inbuilt uncompress - no need to check for command availability

case $(uname -s) in
Darwin) target="macOS_x86_64" ;;
*) target="Linux_x86_64" ;;
esac



if [ $# -eq 0 ]; then
	flyctl_asset_path=$(
		curl -sSf -N https://github.com/superfly/flyctl/releases |
			grep -E -o "/superfly/flyctl/releases/download/.*/flyctl_[0-9]+\\.[0-9]+\\.[0-9]+_${target}.tar.gz" |
			head -n 1
	)
	if [ ! "$flyctl_asset_path" ]; then
		echo "Error: Unable to find latest Flyctl release on GitHub." 1>&2
		exit 1
	fi
	flyctl_uri="https://github.com${flyctl_asset_path}"
else
	if [ "${1}" = "prerel" ]; then
		flyctl_asset_path=$(
		curl -sSf -N https://github.com/superfly/flyctl/releases |
			grep -E -o "/superfly/flyctl/releases/download/.*/flyctl_[0-9]+\\.[0-9]+\\.[0-9]+(\\-beta\\-[0-9]+)*_${target}.tar.gz" |
			head -n 1
		)

		if [ ! "$flyctl_asset_path" ]; then
			echo "Error: Unable to find latest Flyctl release on GitHub." 1>&2
			exit 1
		fi
		flyctl_uri="https://github.com${flyctl_asset_path}"
	else
		flyctl_uri="https://github.com/superfly/flyctl/releases/download/${1}/flyctl-${target}.tar.gz"
	fi
fi

flyctl_install="${FLYCTL_INSTALL:-$HOME/.fly}"

bin_dir="$flyctl_install/bin"
exe="$bin_dir/flyctl"

if [ ! -d "$bin_dir" ]; then
 	mkdir -p "$bin_dir"
fi

curl --fail --location --progress-bar --output "$exe.tar.gz" "$flyctl_uri"
cd "$bin_dir"
tar xzf "$exe.tar.gz"
chmod +x "$exe"
rm "$exe.tar.gz"

if [ "${1}" = "prerel" ]; then
	"$exe" version -s "shell-prerel"
else
	"$exe" version -s "shell"
fi

echo "Flyctl was installed successfully to $exe"
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
