#!/bin/sh

set -e

os=$(uname -s)
case $os in
Darwin) os="macOS" ;;
esac

arch=$(uname -m)

if [ $# -eq 0 ]; then
	asset_path=$(
		command curl -sSf https://github.com/superfly/flyctl/releases |
			command grep -o "/superfly/flyctl/releases/download/.*/flyctl_.*_${os}_${arch}\\.tar\\.gz" |
			command head -n 1
	)
	if [ ! "$asset_path" ]; then exit 1; fi
	binary_uri="https://github.com${asset_path}"
else
	binary_uri="https://github.com/superfly/flyctl/releases/download/v${1}/flyctl_${1}_${os}_${arch}.tar.gz"
fi

bin_dir=${BIN_DIR:-"/usr/local/bin"}
exe="$bin_dir/flyctl"

if [ ! -d "$bin_dir" ]; then
	mkdir -p "$bin_dir"
fi

curl -fL# -o "$exe.tar.gz" "$binary_uri"
tar -xzf "$exe.tar.gz" -C "$bin_dir"
rm "$exe.tar.gz"
chmod +x "$exe"

echo "flyctl was installed successfully to $exe"
if command -v flyctl >/dev/null; then
	echo "Run 'flyctl --help' to get started"
else
	echo "Manually add the directory to your \$HOME/.bash_profile (or similar)"
	echo "  export PATH=\"$bin_dir:\$PATH\""
	echo "Run '$exe --help' to get started"
fi
