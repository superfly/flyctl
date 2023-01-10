package jobs

import (
	"context"
	"fmt"

	"github.com/skratchdot/open-golang/open"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
)

const jobsUrl = "https://fly.io/jobs/"

func NewOpen() *cobra.Command {
	return command.New(
		"open",
		"Open fly.io/jobs",
		"Open browser to https://fly.io/jobs/",
		func(ctx context.Context) error {
			if err := open.Run(jobsUrl); err != nil {
				return fmt.Errorf("failed opening %s: %v", jobsUrl, err)
			}
			return nil
		},
	)
}
