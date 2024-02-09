//go:build windows

package main

import "context"

// handleDebugSignal is no-op on Windows
func handleDebugSignal(_ context.Context) {
}
