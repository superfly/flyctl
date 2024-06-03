package incidents

import (
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"os"
)

// Check for incidents
func Check() bool {
	switch {
	case env.IsTruthy("FLY_INCIDENTS_CHECK"):
		return true
	case env.IsTruthy("FLY_NO_INCIDENTS_CHECK"):
		return false
	case !cmdutil.IsTerminal(os.Stdout), !cmdutil.IsTerminal(os.Stderr):
		return false
	default:
		return true
	}
}
