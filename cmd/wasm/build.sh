#!/usr/bin/env bash
# Builds the browser SSH client into web/: flyssh.wasm plus Go's wasm_exec.js
# loader. Serve web/ from any static server and open index.html.
set -euo pipefail

cd "$(dirname "$0")"

GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o web/flyssh.wasm .
install -m 0644 "$(go env GOROOT)/lib/wasm/wasm_exec.js" web/wasm_exec.js

ls -lh web/flyssh.wasm
