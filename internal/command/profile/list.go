package profile

import (
	"context"
	"fmt"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	fly "github.com/superfly/fly-go"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	profilelib "github.com/superfly/flyctl/internal/profile"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newList() *cobra.Command {
	const (
		short = "List stored credential profiles"
		long  = `Lists every stored profile, the account behind it and where its
credentials live. The profile in effect for the current directory is marked
with an asterisk.

Account names are read from a local cache written at login. Pass --refresh to
re-check every profile against the API, which also reveals expired or revoked
tokens.
`
	)

	cmd := command.New("list", short, long, runList)

	cmd.Aliases = []string{"ls"}

	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Bool{
			Name:        "refresh",
			Description: "Verify each profile's token against the API and update the cached account name",
		},
	)

	return cmd
}

type listEntry struct {
	Name      string `json:"name"`
	Dir       string `json:"config_dir"`
	Email     string `json:"email,omitempty"`
	Status    string `json:"status"`
	Active    bool   `json:"active"`
	LoggedIn  bool   `json:"logged_in"`
	LastLogin string `json:"last_login,omitempty"`
}

func runList(ctx context.Context) error {
	io := iostreams.FromContext(ctx)

	profiles, err := profilelib.List()
	if err != nil {
		return err
	}

	// Mark whichever profile this very command resolved to, so the asterisk
	// reflects the working directory rather than just the `use` pointer.
	var inEffect string
	if res, ok := profilelib.FromContext(ctx); ok && res.Err == nil {
		inEffect = res.Name
	}

	refresh := flag.GetBool(ctx, "refresh")

	entries := make([]listEntry, 0, len(profiles))
	for _, p := range profiles {
		entry := listEntry{
			Name:   p.Name,
			Dir:    p.Dir,
			Email:  p.Metadata.Email,
			Active: p.Name == inEffect,
		}

		cfg, err := config.Load(ctx, filepath.Join(p.Dir, config.FileName))
		switch {
		case err != nil:
			entry.Status = "unreadable"
		case cfg.Tokens == nil || cfg.Tokens.GraphQL() == "":
			entry.Status = "logged out"
		default:
			entry.LoggedIn = true
			entry.Status = "ok"

			if !cfg.LastLogin.IsZero() {
				entry.LastLogin = cfg.LastLogin.Format(time.RFC3339)
			}

			if refresh {
				entry.Email, entry.Status = verify(ctx, cfg.Tokens.GraphQL())

				// Keep the cache honest, so the next plain `list` agrees.
				if entry.Status == "ok" {
					md := p.Metadata
					md.Email = entry.Email
					md.VerifiedAt = time.Now()
					_ = profilelib.WriteMetadata(p.Name, md)
				}
			}
		}

		entries = append(entries, entry)
	}

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(io.Out, entries)
	}

	cs := io.ColorScheme()

	w := tabwriter.NewWriter(io.Out, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "\tNAME\tACCOUNT\tSTATUS\tCONFIG DIRECTORY")

	for _, e := range entries {
		marker := " "
		name := e.Name
		if e.Active {
			marker = "*"
			name = cs.Bold(name)
		}

		email := e.Email
		if email == "" {
			email = "-"
		}

		status := e.Status
		switch status {
		case "ok":
			status = cs.Green(status)
		case "logged out":
			status = cs.Yellow(status)
		default:
			status = cs.Red(status)
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", marker, name, email, status, e.Dir)
	}

	if err := w.Flush(); err != nil {
		return err
	}

	if len(entries) == 1 {
		fmt.Fprintf(io.Out, "\nOnly the default profile exists. Add another account with `fly profile add <name>`.\n")
	}

	// Nothing is marked in effect when resolution failed, which would leave a
	// confusing listing with no asterisk and no explanation.
	if res, ok := profilelib.FromContext(ctx); ok && res.Err != nil {
		fmt.Fprintf(io.ErrOut, "\n%s %s\n", cs.Yellow("warning:"), res.Err)
	}

	return nil
}

// verify trades a token for the account it belongs to, which doubles as a
// liveness check on the credentials.
func verify(ctx context.Context, token string) (email, status string) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	user, err := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{
		AccessToken: token,
	}).GetCurrentUser(ctx)
	if err != nil {
		return "", "invalid token"
	}

	return user.Email, "ok"
}
