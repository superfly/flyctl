package agent

import "errors"

var (
	ErrNoSuchHost        = errors.New("no such host")
	ErrTunnelUnavailable = errors.New("tunnel unavailable")
)
