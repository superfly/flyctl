package tigris

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
)

func destroy() (cmd *cobra.Command) {
	const (
		long = `Permanently destroy a Tigris object storage bucket`

		short = long
		usage = "destroy [storage-bucket-name]"
	)

	cmd = command.New(usage, short, long, runDestroy, command.RequireSession, command.LoadAppNameIfPresent)

	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.App(),
		flag.AppConfig(),
		extensions_core.SharedFlags,
	)

	return cmd
}

func runDestroy(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeTigris)
	if err != nil {
		return err
	}

	if !flag.GetYes(ctx) {
		const msg = "Destroying a Tigris storage bucket is not reversible."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Do you want to destroy the bucket named %s?", extension.Name); {
		case err == nil:
			if !confirmed {
				return nil
			}
		case prompt.IsNonInteractive(err):
			return prompt.NonInteractiveError("yes flag must be specified when not running interactively")
		default:
			return err
		}
	}

	var (
		out    = iostreams.FromContext(ctx).Out
		client = flyutil.ClientFromContext(ctx).GenqClient()
	)

	_, err = gql.DeleteAddOn(ctx, client, extension.Name)

	if err != nil {
		return
	}

	fmt.Fprintf(out, "Your Tigris object storage bucket %s was destroyed\n", extension.Name)

	return
}
