package orgs

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newCrossNetworkReplays() *cobra.Command {
	const (
		long  = `Commands for managing cross-network replay permissions.`
		short = "Manage cross-network replay settings"
	)

	cmd := command.New("cross-network-replays", short, long, nil)

	cmd.AddCommand(
		newCrossNetworkReplaysStatus(),
		newCrossNetworkReplaysEnable(),
		newCrossNetworkReplaysDisable(),
	)

	return cmd
}

func newCrossNetworkReplaysStatus() *cobra.Command {
	const (
		long  = `Show whether cross-network replays are allowed for this organization.`
		short = "Show cross-network replay status"
		usage = "status"
	)

	cmd := command.New(usage, short, long, runCrossNetworkReplaysStatus,
		command.RequireSession,
	)

	flag.Add(cmd, flag.Org(), flag.JSONOutput())

	return cmd
}

func runCrossNetworkReplaysStatus(ctx context.Context) error {
	client := flyutil.ClientFromContext(ctx)

	org, err := OrgFromFlagOrSelect(ctx, fly.AdminOnly)
	if err != nil {
		return err
	}

	allowed, err := client.GetAllowAllCrossNetworkReplays(ctx, org.RawSlug)
	if err != nil {
		return fmt.Errorf("failed to get cross-network replay setting: %w", err)
	}

	io := iostreams.FromContext(ctx)
	cfg := config.FromContext(ctx)

	if cfg.JSONOutput {
		return render.JSON(io.Out, map[string]bool{"allowAllCrossNetworkReplays": allowed})
	}

	if allowed {
		fmt.Fprintf(io.Out, "Cross-network replays are enabled for %s\n", org.RawSlug)
	} else {
		fmt.Fprintf(io.Out, "Cross-network replays are disabled for %s (replays are scoped to networks)\n", org.RawSlug)
	}

	return nil
}

func newCrossNetworkReplaysEnable() *cobra.Command {
	const (
		long  = `Enable cross-network replays for this organization. Apps will be able to replay requests to apps in different networks within the same org.`
		short = "Enable cross-network replays"
		usage = "enable"
	)

	cmd := command.New(usage, short, long, runCrossNetworkReplaysEnable,
		command.RequireSession,
	)

	flag.Add(cmd, flag.Org(), flag.Yes())

	return cmd
}

func runCrossNetworkReplaysEnable(ctx context.Context) error {
	org, err := OrgFromFlagOrSelect(ctx, fly.AdminOnly)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	if !flag.GetYes(ctx) {
		const msg = "Enabling cross-network replays allows apps to replay requests across network boundaries within this org."
		fmt.Fprintln(io.ErrOut, msg)

		switch confirmed, err := prompt.Confirmf(ctx, "Enable cross-network replays for %s?", org.RawSlug); {
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

	client := flyutil.ClientFromContext(ctx)
	_, err = client.SetAllowAllCrossNetworkReplays(ctx, org.RawSlug, true)
	if err != nil {
		return fmt.Errorf("failed to enable cross-network replays: %w", err)
	}

	fmt.Fprintf(io.Out, "Cross-network replays enabled for %s\n", org.RawSlug)

	return nil
}

func newCrossNetworkReplaysDisable() *cobra.Command {
	const (
		long  = `Disable cross-network replays for this organization. Replays will be scoped to apps within the same network.`
		short = "Disable cross-network replays"
		usage = "disable"
	)

	cmd := command.New(usage, short, long, runCrossNetworkReplaysDisable,
		command.RequireSession,
	)

	flag.Add(cmd, flag.Org(), flag.Yes())

	return cmd
}

func runCrossNetworkReplaysDisable(ctx context.Context) error {
	org, err := OrgFromFlagOrSelect(ctx, fly.AdminOnly)
	if err != nil {
		return err
	}

	io := iostreams.FromContext(ctx)

	if !flag.GetYes(ctx) {
		const msg = "Disabling cross-network replays may break existing replay traffic that crosses network boundaries."
		fmt.Fprintln(io.ErrOut, io.ColorScheme().Red(msg))

		switch confirmed, err := prompt.Confirmf(ctx, "Disable cross-network replays for %s?", org.RawSlug); {
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

	client := flyutil.ClientFromContext(ctx)
	_, err = client.SetAllowAllCrossNetworkReplays(ctx, org.RawSlug, false)
	if err != nil {
		return fmt.Errorf("failed to disable cross-network replays: %w", err)
	}

	fmt.Fprintf(io.Out, "Cross-network replays disabled for %s (replays are now scoped to networks)\n", org.RawSlug)

	return nil
}
