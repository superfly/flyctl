package skills

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newRemove() *cobra.Command {
	const (
		short = "Remove the Fly agent skills from local coding agents"
		long  = `Delete the files fly skills install wrote, then any directories that became
empty. Files you added inside a skill directory are left alone. Only agents the
skills were installed into are touched unless --agent is given.`
	)

	cmd := command.New("remove", short, long, runRemove)
	cmd.Args = cobra.NoArgs
	cmd.Aliases = []string{"rm", "uninstall"}

	flag.Add(cmd,
		flag.StringSlice{
			Name:        "agent",
			Description: agentFlagDescription,
		},
	)

	return cmd
}

func runRemove(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	e, err := envFromContext(ctx)
	if err != nil {
		return err
	}

	manifest, err := readManifest(e.configDir)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", manifestPath(e.configDir), err)
	}
	if !manifest.hasInstalledSkills() {
		return errors.New("no Fly skills are installed")
	}

	var targets []Target
	if slugs := flag.GetStringSlice(ctx, "agent"); len(slugs) > 0 {
		tools, err := resolveTools(e.home, slugs)
		if err != nil {
			return err
		}
		targets = buildTargets(tools)
	} else {
		targets = sortedTargetsFromManifest(e, manifest)
	}

	printTargets(ctx, "Removing Fly skills from:", targets)

	removed := 0
	for _, t := range targets {
		key := t.Key()
		skills := manifest.Targets[key]
		if len(skills) == 0 {
			fmt.Fprintf(io.Out, "%s %s: nothing installed\n", colorize.Gray("-"), t.ToolName)
			continue
		}
		for _, name := range sortedKeys(skills) {
			skillDir := joinRel(t.SkillsDir, name)
			files, err := removeSkillFiles(skillDir, skills[name])
			if err != nil {
				return err
			}
			manifest.deleteRecord(key, name)
			removed++
			fmt.Fprintf(io.Out, "%s %s: removed %s %s\n", colorize.SuccessIcon(), colorize.Bold(t.ToolName), colorize.Red(name), colorize.Gray(fmt.Sprintf("(%d file(s))", len(files))))
		}
	}

	if manifest.hasInstalledSkills() {
		if err := manifest.save(e.configDir); err != nil {
			return fmt.Errorf("failed to write %s: %w", manifestPath(e.configDir), err)
		}
	} else if err := os.Remove(manifestPath(e.configDir)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove %s: %w", manifestPath(e.configDir), err)
	}

	fmt.Fprintln(io.Out)
	if removed == 0 {
		fmt.Fprintf(io.Out, "%s No Fly skills were installed in the selected agents.\n", colorize.Yellow("!"))
	} else {
		fmt.Fprintf(io.Out, "%s Removed %d skill(s).\n", colorize.SuccessIcon(), removed)
	}

	return nil
}
