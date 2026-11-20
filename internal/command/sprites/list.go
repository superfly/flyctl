package sprites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

// watchInterval is how often `list --watch` refreshes.
const watchInterval = 2 * time.Second

func newList() *cobra.Command {
	const (
		short = "List sprites."

		long = `List the sprites in the selected organization.

In a terminal this prints a table. When the output is piped it prints one
sprite name per line, for use in scripts. Use --json for full details.
`
	)

	cmd := newCommand("list [flags]", short, long, runList)
	cmd.Args = cobra.NoArgs
	cmd.Aliases = []string{"ls"}
	cmd.Example = `  fly sprites list
  fly sprites list -o my-org --prefix dev
  fly sprites list --json | jq '.sprites[].name'`

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.String{
			Name:        "prefix",
			Description: "Only list sprites whose names start with this prefix",
		},
		flag.Bool{
			Name:        "watch",
			Shorthand:   "w",
			Description: "Refresh the list until interrupted",
		},
	)

	return cmd
}

// spriteJSON is a sprite as printed by --json. The shape matches the
// standalone sprite CLI's output.
type spriteJSON struct {
	ID            string               `json:"id,omitempty"`
	Name          string               `json:"name"`
	Organization  string               `json:"organization,omitempty"`
	Status        string               `json:"status"`
	URL           string               `json:"url,omitempty"`
	URLSettings   *sprites.URLSettings `json:"url_settings,omitempty"`
	Labels        []string             `json:"labels,omitempty"`
	CreatedAt     *time.Time           `json:"created_at,omitempty"`
	UpdatedAt     *time.Time           `json:"updated_at,omitempty"`
	LastRunningAt *time.Time           `json:"last_running_at,omitempty"`
	LastWarmingAt *time.Time           `json:"last_warming_at,omitempty"`
	BucketName    string               `json:"bucket_name,omitempty"`
	PrimaryRegion string               `json:"primary_region,omitempty"`
}

type listJSON struct {
	Sprites []spriteJSON     `json:"sprites"`
	Org     *sprites.OrgInfo `json:"org"`
}

func spriteToJSON(sprite *sprites.Sprite) spriteJSON {
	nonZero := func(t time.Time) *time.Time {
		if t.IsZero() {
			return nil
		}

		return &t
	}

	return spriteJSON{
		ID:            sprite.ID,
		Name:          sprite.Name(),
		Organization:  sprite.OrganizationName,
		Status:        sprite.Status,
		URL:           sprite.URL,
		URLSettings:   sprite.URLSettings,
		Labels:        sprite.Labels,
		CreatedAt:     nonZero(sprite.CreatedAt),
		UpdatedAt:     nonZero(sprite.UpdatedAt),
		LastRunningAt: sprite.LastRunningAt,
		LastWarmingAt: sprite.LastWarmingAt,
		BucketName:    sprite.BucketName,
		PrimaryRegion: sprite.PrimaryRegion,
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(v)
}

func runList(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	var (
		prefix  = flag.GetString(ctx, "prefix")
		jsonOut = flag.GetBool(ctx, "json")
		watch   = flag.GetBool(ctx, "watch")
	)

	if jsonOut && watch {
		return errors.New("--json cannot be combined with --watch")
	}

	if watch {
		return watchList(ctx, s, prefix)
	}

	result, err := s.Client.ListAllSpritesResult(ctx, prefix, nil)
	if err != nil {
		return fmt.Errorf("failed to list sprites: %w", err)
	}

	switch {
	case jsonOut:
		out := listJSON{Sprites: make([]spriteJSON, len(result.Sprites)), Org: result.Org}
		for i, sprite := range result.Sprites {
			out.Sprites[i] = spriteToJSON(sprite)
		}

		return writeJSON(io.Out, out)
	case !io.IsStdoutTTY():
		for _, sprite := range result.Sprites {
			fmt.Fprintln(io.Out, sprite.Name())
		}

		return nil
	}

	return renderList(io, s, result, prefix)
}

func renderList(io *iostreams.IOStreams, s *session, result *sprites.ListResult, prefix string) error {
	if len(result.Sprites) == 0 {
		if prefix != "" {
			fmt.Fprintf(io.Out, "No sprites found with prefix '%s' in organization %s\n", prefix, s.orgDisplay())
		} else {
			fmt.Fprintf(io.Out, "No sprites found in organization %s\n", s.orgDisplay())
		}

		return nil
	}

	fmt.Fprintln(io.Out, listSummary(s, result))

	rows := make([][]string, len(result.Sprites))
	for i, sprite := range result.Sprites {
		url := strings.TrimPrefix(strings.TrimPrefix(sprite.URL, "https://"), "http://")
		if url != "" && urlAuth(sprite) == "public" {
			url += " (public)"
		}

		created := ""
		if !sprite.CreatedAt.IsZero() {
			created = formatAge(time.Since(sprite.CreatedAt)) + " ago"
		}

		lastRunning := ""
		if sprite.LastRunningAt != nil && !strings.EqualFold(sprite.Status, "running") {
			lastRunning = formatAge(time.Since(*sprite.LastRunningAt)) + " ago"
		}

		rows[i] = []string{sprite.Name(), sprite.Status, url, created, lastRunning}
	}

	return render.Table(io.Out, "", rows, "Name", "Status", "URL", "Created", "Last Running")
}

// listSummary reports how many sprites are in each state against the
// organization's limits.
func listSummary(s *session, result *sprites.ListResult) string {
	org := result.Org
	if org == nil {
		org = &sprites.OrgInfo{RunningLimit: len(result.Sprites), WarmLimit: len(result.Sprites)}

		for _, sprite := range result.Sprites {
			switch strings.ToLower(sprite.Status) {
			case "running":
				org.Running++
			case "warm":
				org.Warm++
			case "cold":
				org.Cold++
			}
		}
	}

	name := s.Org
	if name == "" {
		name = org.Name
	}

	return fmt.Sprintf("%s: %d / %d running, %d / %d warm (%d total)",
		name, org.Running, org.RunningLimit, org.Warm, org.WarmLimit, org.Running+org.Warm+org.Cold)
}

// watchList lists the sprites again every watchInterval until the context is
// cancelled.
func watchList(ctx context.Context, s *session, prefix string) error {
	io := iostreams.FromContext(ctx)

	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()

	for {
		result, err := s.Client.ListAllSpritesResult(ctx, prefix, nil)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf("failed to list sprites: %w", err)
		}

		// Redraw in place on a terminal. Anywhere else, such as a pipe to
		// tee, each listing follows the last.
		if io.IsStdoutTTY() {
			fmt.Fprint(io.Out, "\033[H\033[2J")
		}

		if err := renderList(io, s, result, prefix); err != nil {
			return err
		}

		fmt.Fprintf(io.Out, "\nUpdated %s. Press Ctrl+C to stop.\n", time.Now().Format("15:04:05"))

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// formatAge renders a duration at the precision a list needs.
func formatAge(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 48*time.Hour:
		return "1 day"
	default:
		return fmt.Sprintf("%d days", int(d.Hours()/24))
	}
}
