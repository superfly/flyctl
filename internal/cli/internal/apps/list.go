package apps

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/cmd/presenters"
	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/render"
	"github.com/superfly/flyctl/internal/client"
)

func newList() *cobra.Command {
	const (
		long = `The APPS LIST command will show the applications currently
registered and available to this user. The list will include applications 
from all the organizations the user is a member of. Each application will 
be shown with its name, owner and when it was last deployed.
`
		short = "List applications"
	)

	return command.New("list", short, long, runList,
		command.RequireSession,
	)
}

func runList(ctx context.Context) error {
	cfg := config.FromContext(ctx)
	client := client.FromContext(ctx)

	apps, err := client.API().GetApps(ctx, nil)
	if err != nil {
		return err
	}

	p := &presenters.Apps{
		Apps: apps,
	}

	opt := presenters.Options{
		AsJSON: cfg.JSONOutput,
	}

	return render.Presentable(ctx, p, opt)
}
