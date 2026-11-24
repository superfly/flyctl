package incidents

import (
	"os"

	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/iostreams"
)

// Check for incidents
func Check() bool {
	switch {
	case env.IsTruthy("FLY_INCIDENTS_CHECK"):
		return true
	case env.IsTruthy("FLY_NO_INCIDENTS_CHECK"):
		return false
	case !cmdutil.IsTerminal(iostreams.Stdout), !cmdutil.IsTerminal(os.Stderr):
		return false
	default:
		return true
	}
}
