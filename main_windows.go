//go:build windows
// +build windows

package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
)

// todo: embed the dll for the target architecture from deps/wintun
//
//go:embed wintun.dll
var wintunDLL []byte

func init() {
	// todo: only do this if wintun.dll isn't present
	// todo: maybe only do this if wintun.dll isn't present, OR if the existing
	// wintun.dll md5 doesn't match an md5 of the embedded dll calculated at build time
	ex, err := os.Executable()
	if err != nil {
		fmt.Printf("error: find executable path: %v\n", err)
		return
	}
	wintunPath := filepath.Join(filepath.Dir(ex), "wintun.dll")
	err = os.WriteFile(wintunPath, wintunDLL, 0664)
	if err != nil {
		fmt.Printf("error: write wintun.dll file: %v\n", err)
		return
	}
}
