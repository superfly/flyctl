//go:build windows

package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/user"

	"golang.org/x/sys/windows"
)

// Use UNIX sockets since 10.0.17063
// https://devblogs.microsoft.com/commandline/af_unix-comes-to-windows/
func UseUnixSockets() bool {
	maj, _, patch := windows.RtlGetNtVersionNumbers()
	if maj > 10 || maj == 10 && patch >= 17063 {
		return true
	}

	return false
}

func PipeName() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("can't query current username: %w", err)
	}

	name := `\\.\pipe\fly-agent-` + user.Username

	// derive a distinct pipe per socket override so isolated invocations
	// don't share a pipe on systems without unix socket support (the
	// distinguishing part of a generated path is its directory, so hash the
	// whole path rather than taking the basename)
	if socket := SocketPathOverride(); socket != "" {
		sum := sha256.Sum256([]byte(socket))
		name += "-" + hex.EncodeToString(sum[:6])
	}

	return name, nil
}
