package sprites

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newCheckpoint() *cobra.Command {
	const (
		short = "Save and restore a sprite's filesystem state."

		long = `Manage checkpoints, which capture a sprite's writable filesystem overlay.

Restoring a checkpoint replaces the current filesystem state with the
checkpoint's and restarts the sprite's environment. Create a checkpoint first
if you have work you want to keep.
`
	)

	cmd := command.New("checkpoint", short, long, nil)
	cmd.Aliases = []string{"checkpoints"}

	cmd.AddCommand(
		newCheckpointCreate(),
		newCheckpointList(),
		newCheckpointInfo(),
		newCheckpointDelete(),
		newCheckpointRestore(),
	)

	return cmd
}

func newCheckpointCreate() *cobra.Command {
	const (
		short = "Create a checkpoint."
		long  = short + "\n"
	)

	cmd := newCommand("create [flags]", short, long, runCheckpointCreate)
	cmd.Args = cobra.NoArgs

	flag.Add(cmd,
		flag.String{
			Name:        "comment",
			Description: "A comment describing this checkpoint",
		},
	)

	return cmd
}

// progressStream is the common shape of checkpoint and restore progress
// streams.
type progressStream interface {
	Next() (*sprites.StreamMessage, error)
	Close() error
}

// consumeProgress reads a checkpoint or restore stream to its end. The
// operation only counts as done when the server says so: a stream that ends
// early may have been cut off mid-operation.
func consumeProgress(stream progressStream, what string, handle func(*sprites.StreamMessage)) error {
	defer stream.Close() //nolint:errcheck

	for {
		msg, err := stream.Next()
		switch {
		case errors.Is(err, io.EOF):
			return fmt.Errorf("the connection closed before the %s was confirmed; check its state before retrying", what)
		case err != nil:
			return fmt.Errorf("failed to read %s progress: %w", what, err)
		}

		switch msg.Type {
		case "complete":
			return nil
		case "error":
			if msg.Error != "" {
				return fmt.Errorf("%s failed: %s", what, msg.Error)
			}

			return fmt.Errorf("%s failed", what)
		default:
			handle(msg)
		}
	}
}

func runCheckpointCreate(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	name, err := s.spriteName(ctx, "")
	if err != nil {
		return err
	}

	stream, err := s.Client.CreateCheckpointWithComment(ctx, name, flag.GetString(ctx, "comment"))
	if err != nil {
		return fmt.Errorf("failed to create checkpoint: %w", err)
	}

	fmt.Fprintln(io.ErrOut, "Creating checkpoint...")

	var id string

	err = consumeProgress(stream, "checkpoint", func(msg *sprites.StreamMessage) {
		switch msg.Type {
		case "info":
			// The server reports the new checkpoint's ID in an info line.
			if rest, ok := strings.CutPrefix(msg.Data, "  ID: "); ok {
				id = strings.TrimSpace(rest)
			}
		case "stderr":
			fmt.Fprintln(io.ErrOut, msg.Data)
		}
	})
	if err != nil {
		return err
	}

	if id == "" {
		fmt.Fprintf(io.Out, "%s Checkpoint created\n", colorize.SuccessIcon())
		return nil
	}

	fmt.Fprintf(io.Out, "%s Checkpoint %s created\n", colorize.SuccessIcon(), colorize.Bold(id))
	fmt.Fprintf(io.Out, "Restore with: fly sprites restore %s\n", id)

	return nil
}

func newCheckpointList() *cobra.Command {
	const (
		short = "List checkpoints."
		long  = short + "\n"
	)

	cmd := newCommand("list [flags]", short, long, runCheckpointList)
	cmd.Args = cobra.NoArgs
	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.String{
			Name:        "history",
			Description: "Only list checkpoints with this version in their history",
		},
		flag.Bool{
			Name:        "include-auto",
			Description: "Include automatically created checkpoints",
		},
	)

	return cmd
}

func runCheckpointList(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	name, err := s.spriteName(ctx, "")
	if err != nil {
		return err
	}

	checkpoints, err := s.Client.ListCheckpointsWithOptions(ctx, name, sprites.ListCheckpointsOptions{
		HistoryFilter: flag.GetString(ctx, "history"),
		IncludeAuto:   flag.GetBool(ctx, "include-auto"),
	})
	if err != nil {
		return fmt.Errorf("failed to list checkpoints: %w", err)
	}

	if flag.GetBool(ctx, "json") {
		if checkpoints == nil {
			checkpoints = []*sprites.Checkpoint{}
		}

		return writeJSON(io.Out, checkpoints)
	}

	if len(checkpoints) == 0 {
		fmt.Fprintln(io.Out, "No checkpoints found.")
		return nil
	}

	rows := make([][]string, len(checkpoints))
	for i, cp := range checkpoints {
		id := cp.ID

		switch {
		case cp.ID == "Current":
			id = "Current (active)"
		case cp.IsAuto:
			id += " (auto)"
		}

		rows[i] = []string{id, cp.CreateTime.Format("2006-01-02 15:04:05"), cp.Comment}
	}

	return render.Table(io.Out, "", rows, "ID", "Created", "Comment")
}

func newCheckpointInfo() *cobra.Command {
	const (
		short = "Show a checkpoint's details."
		long  = short + "\n"
	)

	cmd := newCommand("info [flags] <version-id>", short, long, runCheckpointInfo)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Example = `  fly sprites checkpoint info v2`

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runCheckpointInfo(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	name, err := s.spriteName(ctx, "")
	if err != nil {
		return err
	}

	cp, err := s.Client.GetCheckpoint(ctx, name, flag.FirstArg(ctx))
	if err != nil {
		return fmt.Errorf("failed to get checkpoint info: %w", err)
	}

	if flag.GetBool(ctx, "json") {
		return writeJSON(io.Out, cp)
	}

	fmt.Fprintf(io.Out, "ID: %s\n", cp.ID)
	fmt.Fprintf(io.Out, "Created: %s\n", cp.CreateTime.Format(time.RFC3339))

	if strings.TrimSpace(cp.Comment) != "" {
		fmt.Fprintf(io.Out, "Comment: %s\n", cp.Comment)
	}

	if len(cp.History) == 0 {
		fmt.Fprintln(io.Out, "History: (none)")
		return nil
	}

	fmt.Fprintln(io.Out, "History:")
	for _, entry := range cp.History {
		fmt.Fprintf(io.Out, "  %s\n", entry)
	}

	return nil
}

func newCheckpointDelete() *cobra.Command {
	const (
		short = "Delete a checkpoint."
		long  = short + "\n"
	)

	cmd := newCommand("delete [flags] <version-id>", short, long, runCheckpointDelete)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Aliases = []string{"rm"}
	cmd.Example = `  fly sprites checkpoint delete v3`

	return cmd
}

func runCheckpointDelete(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	name, err := s.spriteName(ctx, "")
	if err != nil {
		return err
	}

	id := flag.FirstArg(ctx)

	resp, err := s.request(ctx, http.MethodDelete,
		"/v1/sprites/"+url.PathEscape(name)+"/checkpoints/"+url.PathEscape(id))
	if err != nil {
		return fmt.Errorf("failed to delete checkpoint: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		fmt.Fprintf(io.Out, "%s Checkpoint %s deleted\n", colorize.SuccessIcon(), colorize.Bold(id))
		return nil
	case http.StatusNotFound:
		return fmt.Errorf("checkpoint %s not found", id)
	case http.StatusConflict:
		return flyerr.WithSuggestion(
			fmt.Errorf("cannot delete checkpoint %s: it is the active checkpoint", id),
			"Restore a different checkpoint first, then delete this one.",
		)
	default:
		return responseError(resp)
	}
}

// responseError turns an unexpected API response into an error.
func responseError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if apiErr := sprites.ParseAPIError(resp, body); apiErr != nil {
		return apiErr
	}

	return fmt.Errorf("the Sprites API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

const restoreLong = `Restore a sprite's filesystem from a checkpoint.

Restoring replaces the sprite's writable filesystem overlay with the state in
the checkpoint and restarts its environment, which ends active sessions. The
current state cannot be recovered afterwards unless you checkpoint it first:

  fly sprites checkpoint create --comment "before rollback"
  fly sprites restore v1
`

func newCheckpointRestore() *cobra.Command {
	cmd := newCommand("restore [flags] <version-id>", "Restore from a checkpoint.", restoreLong, runRestore)
	cmd.Args = cobra.ExactArgs(1)

	return cmd
}

func newRestore() *cobra.Command {
	cmd := newCommand("restore [flags] <version-id>", "Restore a sprite from a checkpoint.", restoreLong, runRestore)
	cmd.Args = cobra.ExactArgs(1)
	cmd.Example = `  fly sprites restore v1
  fly sprites restore -o my-org -s my-sprite v2`

	return cmd
}

func runRestore(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	name, err := s.spriteName(ctx, "")
	if err != nil {
		return err
	}

	id := flag.FirstArg(ctx)

	stream, err := s.Client.RestoreCheckpoint(ctx, name, id)
	if err != nil {
		return fmt.Errorf("failed to restore checkpoint: %w", err)
	}

	fmt.Fprintf(io.ErrOut, "Restoring checkpoint %s...\n", id)

	err = consumeProgress(stream, "restore", func(msg *sprites.StreamMessage) {
		switch msg.Type {
		case "stderr":
			fmt.Fprintln(io.ErrOut, msg.Data)
		case "warning":
			fmt.Fprintf(io.ErrOut, "Warning: %s\n", msg.Data)
		}
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "%s Restored to checkpoint %s\n", colorize.SuccessIcon(), colorize.Bold(id))

	return nil
}
