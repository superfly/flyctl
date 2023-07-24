package launch

import (
	"context"

	"github.com/superfly/flyctl/iostreams"
)

// Launch launches the app described by the plan. This is the main entry point for launching a plan.
func (state *launchState) Launch(ctx context.Context) error {

	io := iostreams.FromContext(ctx)

	// TODO(Allison): are we still supporting the launch-into usecase?
	// I'm assuming *not* for now, because it's confusing UX and this
	// is the perfect time to remove it.

	return nil
}
