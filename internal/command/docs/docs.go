// Package docs implements the docs command chain.
package docs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/skratchdot/open-golang/open"
)

func New() (cmd *cobra.Command) {
	const (
		long = `View Fly documentation on the Fly.io website. This command will open a
browser to view the content.
`
		short = "View Fly documentation"
	)

	cmd = command.New("docs", short, long, run)

	cmd.Args = cobra.NoArgs

	return
}

func run(ctx context.Context) error {
	const url = "https://fly.io/docs/"

	out := iostreams.FromContext(ctx).ErrOut
	fmt.Fprintf(out, "opening %s ...\n", url)

	if err := open.Run(url); err != nil {
		return fmt.Errorf("failed opening %s: %w", url, err)
	}

	return nil
}
