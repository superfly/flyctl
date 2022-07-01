package agent

import "errors"

var (
	ErrNoSuchHost        = errors.New("host was not found in DNS")
	ErrTunnelUnavailable = errors.New("tunnel unavailable")
)
