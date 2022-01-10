package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"

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
	client, err := newClient(ctx)
	if err != nil {
		return err
	}

	status, err := client.Status(ctx)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	}
	if err != nil {
		return fmt.Errorf("failed pinging: %w", err)
	}

	cfg := config.FromContext(ctx)
	out := iostreams.FromContext(ctx).Out

	if cfg.JSONOutput {
		_ = render.JSON(out, status)

		return nil
	}

	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%-10s: %d\n", "PID", status.PID)
	fmt.Fprintf(&buf, "%-10s: %s\n", "Version", status.Version.String())
	fmt.Fprintf(&buf, "%-10s: %t\n", "Background", status.Background)

	buf.WriteTo(out)

	return nil
}
