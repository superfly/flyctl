package agent

import (
	"bytes"
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/pkg/iostreams"

	"github.com/superfly/flyctl/internal/cli/internal/command"
	"github.com/superfly/flyctl/internal/cli/internal/config"
	"github.com/superfly/flyctl/internal/cli/internal/render"
)

func newPing() *cobra.Command {
	const (
		short = "Ping the Fly agent"
		long  = short + "\n"
	)

	return command.New("ping", short, long, runPing,
		command.RequireSession,
	)
}

func runPing(ctx context.Context) error {
	client, err := fetchClient(ctx)
	if err != nil {
		return err
	}

	res, err := client.Ping(ctx)
	if err != nil {
		return fmt.Errorf("failed pinging: %w", err)
	}

	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		_ = render.JSON(out, res)

		return nil
	}

	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%-10s: %d\n", "PID", res.PID)
	fmt.Fprintf(&buf, "%-10s: %s\n", "Version", res.Version.String())
	fmt.Fprintf(&buf, "%-10s: %t\n", "Background", res.Background)

	buf.WriteTo(out)

	return nil
}
