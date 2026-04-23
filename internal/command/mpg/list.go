package mpg

import (
	"context"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/internal/command"
	cmdv1 "github.com/superfly/flyctl/internal/command/mpg/v1"
	"github.com/superfly/flyctl/internal/command/orgs"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex"
)

func newList() *cobra.Command {
	const (
		long = `List MPG clusters owned by the specified organization.
If no organization is specified, the user's personal organization is used.`
		short = "List MPG clusters."
		usage = "list"
	)

	cmd := command.New(usage, short, long, runList,
		command.RequireSession,
	)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd, flag.JSONOutput())
	flag.Add(cmd, flag.Org())
	flag.Add(cmd, flag.Bool{
		Name:        "deleted",
		Description: "Show deleted clusters instead of active clusters",
		Default:     false,
	})

	return cmd
}

func runList(ctx context.Context) error {
	org, err := orgs.OrgFromFlagOrSelect(ctx)
	if err != nil {
		return err
	}

	return cmdv1.RunList(ctx, org.Slug)
}

// formatAttachedApps formats the list of attached apps for display.
// Delegates to the v1 implementation.
func formatAttachedApps(apps []uiex.AttachedApp) string {
	return cmdv1.FormatAttachedApps(apps)
}
