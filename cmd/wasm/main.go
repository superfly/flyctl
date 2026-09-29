//go:build js && wasm

// Command wasm is a browser build of flyctl's SSH and SFTP client.
// Build with `GOOS=js GOARCH=wasm go build -trimpath -ldflags="-s -w" -o web/flyssh.wasm .`
//
// It installs a JavaScript API on globalThis.flySSH, declared in
// web/flyssh.d.ts; each entry point in this package names the declared
// signature it implements.
package main

import (
	"log"
	"syscall/js"
)

func main() {
	log.SetFlags(0)

	js.Global().Set("flySSH", js.ValueOf(map[string]any{
		"generateKey": js.FuncOf(generateKey),
		"connect":     js.FuncOf(connect),
	}))

	select {}
}
