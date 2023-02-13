package platform

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() (cmd *cobra.Command) {
	const (
		long = `Show current Fly platform status in a browser
`
		short = "Show current platform status"
	)

	cmd = command.New("status", short, long, runStatus)

	cmd.Args = cobra.NoArgs

	return
}

func runStatus(ctx context.Context) error {
	const url = "https://status.fly.io/"

	w := iostreams.FromContext(ctx).ErrOut
	fmt.Fprintf(w, "opening %s ...\n", url)

	if err := open.Run(url); err != nil {
		return fmt.Errorf("failed opening %s: %w", url, err)
	}

	return nil
}
