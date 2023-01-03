package jobs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/iostreams"
)

func New() *cobra.Command {
	const (
		short = "Show jobs at Fly.io"

		long = `Show jobs at Fly.io, including maybe ones you should apply to`
	)

	cmd := command.New("jobs", short, long, run)
	cmd.AddCommand(NewOpen())
	return cmd
}
func run(ctx context.Context) (err error) {
	out := iostreams.FromContext(ctx).Out
	_, err = fmt.Fprintln(out, `Want to work on super fun problems with (arguably) likeable people? Then you’ve come to the right place.

The tl;dr is that we build on Rust, Go, Ruby, and Elixir, on Linux. If you're comfortable with any of those, we probably have interesting roles for you.

We've got roles on our API backend, defining our developer experience; on our Elixir frontend; in security engineering; on infrastructure; and, of course, on the platform itself.

Check out https://fly.io/jobs to see our open roles. Or run: fly jobs open`)

	if err != nil {
		return err
	}
	return nil
}
