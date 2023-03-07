package appsv2

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newShow() *cobra.Command {
	cmd := command.New(
		`show <org-slug>`,
		`Show apps v2 default for org`,
		`Show whether apps v2 is default on or off for the org`,
		runShow,
		command.RequireSession,
	)
	cmd.Args = cobra.ExactArgs(1)
	return cmd
}

func runShow(ctx context.Context) error {
	var (
		cfg       = config.FromContext(ctx)
		io        = iostreams.FromContext(ctx)
		colorize  = io.ColorScheme()
		apiClient = client.FromContext(ctx).API()
		orgSlug   = flag.FirstArg(ctx)
	)
	appsV2DefaultOn, err := apiClient.GetAppsV2DefaultOnForOrg(ctx, orgSlug)
	if err != nil {
		return fmt.Errorf("failed to get apps v2 setting due to error: %w", err)
	}
	if cfg.JSONOutput {
		fmt.Fprintf(io.Out, `{"apps_v2_default_on": %t}`, appsV2DefaultOn)
	} else {
		fmt.Fprintf(io.Out, "Apps V2 default on: %s\n", colorize.Bold(fmt.Sprintf("%t", appsV2DefaultOn)))
	}
	return nil
}
