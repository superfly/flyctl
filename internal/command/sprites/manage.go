package sprites

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	sprites "github.com/superfly/sprites-go"

	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

// spriteNameRegex matches the names the Sprites API accepts.
var spriteNameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)

func validateSpriteName(name string) error {
	switch {
	case len(name) < 3:
		return errors.New("sprite name must be at least 3 characters")
	case len(name) > 55:
		return errors.New("sprite name must be at most 55 characters")
	case !spriteNameRegex.MatchString(name):
		return errors.New("sprite name must contain only lowercase letters, numbers, and hyphens, and start and end with a letter or number")
	}

	return nil
}

func newCreate() *cobra.Command {
	const (
		short = "Create a new sprite."

		long = `Create a new sprite in the selected organization.

A sprite runs a base Ubuntu image with a writable overlay that holds your
changes. It pauses when idle and resumes on demand.

After creating the sprite, this command opens a console in it unless
--skip-console is passed or the command is not running in a terminal.
`
	)

	cmd := newCommand("create [flags] [sprite-name]", short, long, runCreate)
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Example = `  fly sprites create my-sprite
  fly sprites create -o my-org dev-sprite --label env:dev
  fly sprites create my-sprite --skip-console`

	flag.Add(cmd,
		flag.Bool{
			Name:        "skip-console",
			Description: "Exit after creating the sprite instead of opening a console",
		},
		flag.StringArray{
			Name:        "label",
			Description: "Label to apply to the sprite (can be repeated)",
		},
	)

	return cmd
}

func runCreate(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	name := flag.FirstArg(ctx)
	if name == "" {
		name = flag.GetString(ctx, spriteFlagName)
	}

	if name == "" {
		if err := prompt.String(ctx, &name, "Sprite name:", "", true); err != nil {
			if prompt.IsNonInteractive(err) {
				return errors.New("a sprite name is required when not running interactively")
			}

			return err
		}
	}

	if err := validateSpriteName(name); err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Creating sprite %s in organization %s...\n", colorize.Bold(name), s.orgDisplay())

	start := time.Now()

	sprite, err := s.Client.CreateSpriteWithOrg(ctx, name, nil, nil, flag.GetStringArray(ctx, "label"))
	if err != nil {
		return fmt.Errorf("failed to create sprite: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Created sprite %s in %.1fs\n",
		colorize.SuccessIcon(), colorize.Bold(sprite.Name()), time.Since(start).Seconds())

	// The create call returns only the name. The URL is worth a lookup, but
	// not worth failing over.
	if created, err := s.Client.GetSprite(ctx, sprite.Name()); err == nil && created.URL != "" {
		fmt.Fprintf(io.Out, "  URL: %s\n", created.URL)
	}

	if flag.GetBool(ctx, "skip-console") || !io.IsInteractive() {
		return nil
	}

	fmt.Fprintln(io.Out, "Connecting to console...")

	return openConsole(ctx, sprite, true)
}

func newDestroy() *cobra.Command {
	const (
		short = "Destroy a sprite."

		long = `Permanently destroy a sprite and all of its data, including its checkpoints.
This cannot be undone.
`
	)

	cmd := newCommand("destroy [flags] [sprite-name]", short, long, runDestroy)
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"delete", "rm"}

	flag.Add(cmd,
		flag.Yes(),
		flag.Bool{
			Name:        "force",
			Description: "Skip the confirmation prompt (same as --yes)",
		},
	)

	return cmd
}

func runDestroy(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	// Destroying is irreversible, so require the sprite to be named or set for
	// the directory rather than picked from a list.
	name, err := s.namedSprite(ctx, flag.FirstArg(ctx))
	if err != nil {
		return err
	}

	if !flag.GetYes(ctx) && !flag.GetBool(ctx, "force") {
		msg := fmt.Sprintf("Destroy sprite %s in organization %s? This permanently deletes the sprite and all its data.",
			name, s.orgDisplay())

		switch confirmed, err := prompt.Confirm(ctx, msg); {
		case prompt.IsNonInteractive(err):
			return flyerr.WithSuggestion(
				errors.New("destroying a sprite needs confirmation"),
				"Pass --yes to confirm when not running interactively.",
			)
		case err != nil:
			return err
		case !confirmed:
			fmt.Fprintln(io.Out, "Cancelled.")
			return nil
		}
	}

	if err := s.Client.DeleteSprite(ctx, name); err != nil {
		return fmt.Errorf("failed to destroy sprite: %w", err)
	}

	// Drop the directory's .sprite file if it pointed at this sprite.
	wd := state.WorkingDirectory(ctx)
	if sf, path, _ := readSpriteFile(wd); sf != nil && sf.Sprite == name && (sf.Organization == "" || sf.Organization == s.Org) {
		if path == spriteFileWritePath(wd) {
			if err := removeSpriteFile(wd); err != nil {
				fmt.Fprintf(io.ErrOut, "Warning: failed to remove %s: %v\n", path, err)
			}
		}
	}

	fmt.Fprintf(io.Out, "%s Sprite %s destroyed\n", colorize.SuccessIcon(), colorize.Bold(name))

	return nil
}

func newUse() *cobra.Command {
	const (
		short = "Set the sprite used in the current directory."

		long = `Write a .sprite file in the current directory that names a sprite and its
organization. Other sprites commands run in this directory, or below it, use
that sprite when --sprite is not passed.

The file is the same one the standalone sprite CLI reads and writes.
`
	)

	cmd := newCommand("use [flags] [sprite-name]", short, long, runUse)
	cmd.Args = cobra.MaximumNArgs(1)

	flag.Add(cmd,
		flag.Bool{
			Name:        "unset",
			Description: "Remove the .sprite file from the current directory",
		},
	)

	// --unset only touches a local file, so it must work logged out.
	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if unset, _ := cmd.Flags().GetBool("unset"); unset {
			return runUnset(cmd)
		}

		return run(cmd, args)
	}

	return cmd
}

func runUnset(cmd *cobra.Command) error {
	if err := removeSpriteFile("."); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return errors.New("no .sprite file found in the current directory")
		}

		return fmt.Errorf("failed to remove the .sprite file: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Removed the .sprite file from the current directory")

	return nil
}

func runUse(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	name := flag.FirstArg(ctx)
	if name == "" {
		name = flag.GetString(ctx, spriteFlagName)
	}

	if name == "" {
		var err error
		if name, err = s.selectSprite(ctx); err != nil {
			if prompt.IsNonInteractive(err) {
				return errors.New("a sprite name is required when not running interactively")
			}

			return err
		}
	} else if _, err := s.Client.GetSprite(ctx, name); err != nil {
		return fmt.Errorf("failed to find sprite %s in organization %s: %w", name, s.orgDisplay(), err)
	}

	if err := writeSpriteFile(state.WorkingDirectory(ctx), s.Org, name); err != nil {
		return fmt.Errorf("failed to write the .sprite file: %w", err)
	}

	fmt.Fprintf(io.Out, "%s Now using sprite %s from organization %s\n",
		colorize.SuccessIcon(), colorize.Bold(name), s.orgDisplay())
	fmt.Fprintln(io.Out, "\nOther sprites commands in this directory will use this sprite by default.")
	fmt.Fprintln(io.Out, "To unset, run: fly sprites use --unset")

	return nil
}

func newInfo() *cobra.Command {
	const (
		short = "Show a sprite's details, including its URL and labels."
		long  = short + "\n"
	)

	cmd := newCommand("info [flags] [sprite-name]", short, long, runInfo)
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Aliases = []string{"status"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runInfo(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	name, err := s.spriteName(ctx, flag.FirstArg(ctx))
	if err != nil {
		return err
	}

	sprite, err := s.Client.GetSprite(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get sprite info: %w", err)
	}

	if flag.GetBool(ctx, "json") {
		return writeJSON(io.Out, spriteToJSON(sprite))
	}

	field := func(label, value string) {
		if value != "" {
			fmt.Fprintf(io.Out, "%-16s %s\n", label+":", value)
		}
	}

	org := sprite.OrganizationName
	if org == "" {
		org = s.Org
	}

	field("Name", sprite.Name())
	field("Organization", org)
	field("ID", sprite.ID)
	field("Status", sprite.Status)
	field("Runtime version", s.Client.SpriteVersion())

	if !sprite.CreatedAt.IsZero() {
		field("Created", sprite.CreatedAt.Format(time.RFC3339))
	}

	if sprite.LastRunningAt != nil && !sprite.LastRunningAt.IsZero() {
		field("Last running", sprite.LastRunningAt.Format(time.RFC3339))
	}

	if sprite.URL == "" {
		fmt.Fprintf(io.Out, "No URL available for sprite %s\n", sprite.Name())
	} else {
		field("URL", sprite.URL)
		field("Auth", urlAuth(sprite))
	}

	field("Labels", strings.Join(sprite.Labels, ", "))

	return nil
}

// urlAuth returns who may reach the sprite's URL. "sprite", the default,
// restricts it to members of the organization.
func urlAuth(sprite *sprites.Sprite) string {
	if sprite.URLSettings != nil && sprite.URLSettings.Auth != "" {
		return sprite.URLSettings.Auth
	}

	return "sprite"
}

func newConfig() *cobra.Command {
	const (
		short = "Show or update a sprite's configuration."

		long = `Show a sprite's URL authentication, private access setting and labels.
Use 'fly sprites config update' to change them.
`
	)

	cmd := newCommand("config [flags] [sprite-name]", short, long, runConfigShow)
	cmd.Args = cobra.MaximumNArgs(1)

	cmd.AddCommand(newConfigUpdate())

	return cmd
}

func runConfigShow(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	name, err := s.spriteName(ctx, flag.FirstArg(ctx))
	if err != nil {
		return err
	}

	sprite, err := s.Client.GetSprite(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get sprite config: %w", err)
	}

	privateAccess := "admins"
	if sprite.URLSettings != nil && sprite.URLSettings.PrivateAccess != "" {
		privateAccess = sprite.URLSettings.PrivateAccess
	}

	labels := "(none)"
	if len(sprite.Labels) > 0 {
		labels = strings.Join(sprite.Labels, ", ")
	}

	fmt.Fprintf(io.Out, "url-auth:       %s\n", urlAuth(sprite))
	fmt.Fprintf(io.Out, "private-access: %s\n", privateAccess)
	fmt.Fprintf(io.Out, "labels:         %s\n", labels)

	return nil
}

func newConfigUpdate() *cobra.Command {
	const (
		short = "Update a sprite's configuration."

		long = `Update a sprite's URL authentication, private access setting or labels.

--url-auth is "public" (no authentication) or "sprite" (organization members
only). --private-access applies to private URLs and is "admins" or "org_users".
`
	)

	cmd := newCommand("update [flags] [sprite-name]", short, long, runConfigUpdate)
	cmd.Args = cobra.MaximumNArgs(1)
	cmd.Example = `  fly sprites config update --url-auth public
  fly sprites config update my-sprite --label env:prod --label team:web
  fly sprites config update --clear-labels`

	flag.Add(cmd,
		flag.String{
			Name:        "url-auth",
			Description: `URL authentication: "public" or "sprite"`,
		},
		flag.String{
			Name:        "private-access",
			Description: `Who can reach a private sprite URL: "admins" or "org_users"`,
		},
		flag.StringArray{
			Name:        "label",
			Description: "Label to apply to the sprite (can be repeated)",
		},
		flag.Bool{
			Name:        "clear-labels",
			Description: "Clear all existing labels before applying --label",
		},
	)

	return cmd
}

func runConfigUpdate(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	var (
		auth          = flag.GetString(ctx, "url-auth")
		privateAccess = flag.GetString(ctx, "private-access")
		labels        = flag.GetStringArray(ctx, "label")
		clearLabels   = flag.GetBool(ctx, "clear-labels")
	)

	switch {
	case auth == "" && privateAccess == "" && len(labels) == 0 && !clearLabels:
		return errors.New("at least one of --url-auth, --private-access, --label, or --clear-labels is required")
	case auth != "" && auth != "public" && auth != "sprite":
		return fmt.Errorf(`invalid --url-auth %q: must be "public" or "sprite"`, auth)
	case privateAccess != "" && privateAccess != "admins" && privateAccess != "org_users":
		return fmt.Errorf(`invalid --private-access %q: must be "admins" or "org_users"`, privateAccess)
	case auth == "public" && privateAccess != "":
		return errors.New(`--private-access only applies when --url-auth is "sprite" (private URLs)`)
	}

	name, err := s.spriteName(ctx, flag.FirstArg(ctx))
	if err != nil {
		return err
	}

	req := &sprites.UpdateSpriteRequest{
		Labels:      labels,
		ClearLabels: clearLabels,
	}

	if auth != "" || privateAccess != "" {
		req.URLSettings = &sprites.URLSettings{Auth: auth, PrivateAccess: privateAccess}
	}

	if err := s.Client.UpdateSprite(ctx, name, req); err != nil {
		return fmt.Errorf("failed to update sprite: %w", err)
	}

	fmt.Fprintf(io.Out, "Updated %s\n", name)

	return nil
}

func newURL() *cobra.Command {
	const (
		short = "Show a sprite's URL."

		long = `Show a sprite's always-on HTTP URL and who may reach it.

To change who may reach it, use 'fly sprites config update --url-auth'.
`
	)

	cmd := newCommand("url [flags] [sprite-name]", short, long, runURL)
	cmd.Args = cobra.MaximumNArgs(1)

	cmd.AddCommand(newURLUpdate())

	return cmd
}

func runURL(ctx context.Context, s *session) error {
	name, err := s.spriteName(ctx, flag.FirstArg(ctx))
	if err != nil {
		return err
	}

	return printURL(ctx, s, name, true)
}

func printURL(ctx context.Context, s *session, name string, withAuth bool) error {
	io := iostreams.FromContext(ctx)

	sprite, err := s.Client.GetSprite(ctx, name)
	if err != nil {
		return fmt.Errorf("failed to get sprite info: %w", err)
	}

	if sprite.URL == "" {
		fmt.Fprintf(io.Out, "No URL available for sprite %s\n", name)
		return nil
	}

	fmt.Fprintf(io.Out, "URL: %s\n", sprite.URL)

	if withAuth {
		fmt.Fprintf(io.Out, "Auth: %s\n", urlAuth(sprite))
	}

	return nil
}

func newURLUpdate() *cobra.Command {
	const (
		short = "Update a sprite's URL authentication."
		long  = short + "\n"
	)

	cmd := newCommand("update --auth <type> [flags]", short, long, runURLUpdate)
	cmd.Args = cobra.NoArgs
	cmd.Deprecated = "use 'fly sprites config update --url-auth' instead"

	flag.Add(cmd,
		flag.String{
			Name:        "auth",
			Shorthand:   "a",
			Description: `Authentication type: "public" or "sprite"`,
		},
	)

	return cmd
}

func runURLUpdate(ctx context.Context, s *session) error {
	io := iostreams.FromContext(ctx)

	auth := flag.GetString(ctx, "auth")

	switch auth {
	case "":
		return errors.New("--auth is required")
	case "default":
		auth = "sprite"
	case "public", "sprite":
	default:
		return fmt.Errorf(`invalid --auth %q: must be "public" or "sprite"`, auth)
	}

	name, err := s.spriteName(ctx, "")
	if err != nil {
		return err
	}

	if err := s.Client.UpdateURLSettings(ctx, name, &sprites.URLSettings{Auth: auth}); err != nil {
		return fmt.Errorf("failed to update URL settings: %w", err)
	}

	fmt.Fprintf(io.Out, "Updated URL settings for sprite %s\n", name)
	fmt.Fprintf(io.Out, "Auth: %s\n", auth)

	return printURL(ctx, s, name, false)
}
