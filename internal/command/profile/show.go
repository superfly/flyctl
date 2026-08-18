package profile

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	profilelib "github.com/superfly/flyctl/internal/profile"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/iostreams"
)

func newShow() *cobra.Command {
	const (
		short = "Show which profile is in effect, and why"
		long  = `Reports the profile the current directory and environment
resolve to, the config directory backing it, and which rule selected it. Use
this to confirm which account a command is about to touch.
`
	)

	cmd := command.New("show", short, long, runShow)

	cmd.Aliases = []string{"which", "current"}

	flag.Add(cmd, flag.JSONOutput())

	return cmd
}

func runShow(ctx context.Context) error {
	io := iostreams.FromContext(ctx)

	// The resolution that selected this command's own config directory is the
	// answer, so report it rather than resolving a second time.
	res, ok := profilelib.FromContext(ctx)
	if !ok {
		return fmt.Errorf("profile was not resolved for this command")
	}

	cs := io.ColorScheme()

	name := res.Name
	if name == "" {
		name = "(none)"
	}

	selectedBy := string(res.Source)
	if res.Detail != "" {
		selectedBy = fmt.Sprintf("%s (%s)", res.Source, res.Detail)
	}

	var md profilelib.Metadata
	if res.Name != "" {
		md, _ = profilelib.ReadMetadata(res.Name)
	}

	// Resolution only fails this far in on the tolerated path, where the
	// answer is not "which profile" but "why is there no usable one".
	if res.Err != nil {
		if config.FromContext(ctx).JSONOutput {
			return render.JSON(io.Out, map[string]string{
				"profile": "",
				"error":   res.Err.Error(),
			})
		}

		fmt.Fprintf(io.Out, "%s %s\n", cs.Red("unresolved:"), res.Err)
		fmt.Fprintf(io.Out, "\nFix it by creating the profile, or by pointing at one that exists:\n")
		fmt.Fprintf(io.Out, "  fly profile list\n")
		fmt.Fprintf(io.Out, "  fly profile use <name>\n")
		fmt.Fprintf(io.Out, "  fly profile link <name>   # rewrites %s here\n", profilelib.ProjectFileName)

		return nil
	}

	if config.FromContext(ctx).JSONOutput {
		return render.JSON(io.Out, map[string]string{
			"profile":     name,
			"config_dir":  res.Dir,
			"selected_by": selectedBy,
			"email":       md.Email,
		})
	}

	w := tabwriter.NewWriter(io.Out, 0, 0, 2, ' ', 0)

	fmt.Fprintf(w, "Profile\t%s\n", cs.Bold(name))
	fmt.Fprintf(w, "Config directory\t%s\n", res.Dir)
	fmt.Fprintf(w, "Selected by\t%s\n", selectedBy)

	if md.Email != "" {
		fmt.Fprintf(w, "Account\t%s\n", md.Email)
	}

	// A pinned config dir means profiles are out of the picture entirely;
	// saying so avoids a confusing "(none)" with no explanation.
	if res.Source == profilelib.SourceConfigDirEnv {
		fmt.Fprintf(w, "Note\t%s is set, so profile selection is bypassed\n", profilelib.ConfigDirEnvKey)
	}

	return w.Flush()
}
