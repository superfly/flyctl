package launch

import (
	"context"

	"github.com/superfly/flyctl/scanner"
)

func v2Launch(ctx context.Context, plan *launchPlan, srcInfo *scanner.SourceInfo) error {

	// TODO(Allison): are we still supporting the launch-into usecase?
	// I'm assuming *not* for now, because it's confusing UX and this
	// is the perfect time to remove it.

	return nil
}
