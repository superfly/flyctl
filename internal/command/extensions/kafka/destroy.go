package kafka

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/command"
	extensions_core "github.com/superfly/flyctl/internal/command/extensions/core"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func destroy() (cmd *cobra.Command) {
	const (
		long = `Permanently destroy an Upstash Kafka cluster`

		short = long
		usage = "destroy [name]"
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

	extension, _, err := extensions_core.Discover(ctx, gql.AddOnTypeUpstashKafka)
	if err != nil {
		return err
	}

	if !flag.GetYes(ctx) {
		const msg = "Destroying an upstash Kafka cluster is not reversible."
		fmt.Fprintln(io.ErrOut, colorize.Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Do you want to destroy the cluster named %s?", extension.Name); {
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

	client := flyutil.ClientFromContext(ctx).GenqClient()
	if _, err := gql.DeleteAddOn(ctx, client, extension.Name); err != nil {
		return err
	}

	out := iostreams.FromContext(ctx).Out
	fmt.Fprintf(out, "Your Upstash Kafka cluster %s was destroyed\n", extension.Name)

	return nil
}
