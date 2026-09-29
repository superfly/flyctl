//go:build !js

package wg

import (
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/terminal"
	"golang.zx2c4.com/wireguard/device"
)

// The CLI drives wireguard-go's verbosity from flyctl's own log level.
func init() {
	deviceLogLevel = func() int {
		if terminal.GetLogLevel() == logger.Debug {
			return device.LogLevelVerbose
		}

		return device.LogLevelError
	}
}
