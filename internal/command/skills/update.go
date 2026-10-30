package skills

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
)

func newUpdate() *cobra.Command {
	const (
		short = "Update the installed Fly agent skills to the latest revision"
		long  = `Re-download the Fly agent skills and refresh every file that is still as
installed. Files you have edited are left alone and listed; pass --force to
overwrite them. Only agents the skills were previously installed into are
updated unless --agent is given.`
	)

	cmd := command.New("update", short, long, runUpdate)
	cmd.Args = cobra.NoArgs
	cmd.Aliases = []string{"upgrade"}

	flag.Add(cmd,
		flag.StringSlice{
			Name:        "agent",
			Description: AgentFlagDescription(),
		},
		flag.Bool{
			Name:        "force",
			Description: "Overwrite files you have modified locally instead of skipping them",
		},
	)
	addSourceFlags(cmd, true)

	return cmd
}

func runUpdate(ctx context.Context) error {
	e, err := envFromContext(ctx)
	if err != nil {
		return err
	}

	var targets []Target
	if slugs := flag.GetStringSlice(ctx, "agent"); len(slugs) > 0 {
		tools, err := resolveTools(e.home, slugs)
		if err != nil {
			return err
		}
		targets = buildTargets(tools)
	} else {
		manifest, err := readManifest(e.configDir)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", manifestPath(e.configDir), err)
		}
		if !manifest.hasInstalledSkills() {
			return errors.New("no Fly skills are installed; run `fly skills install` first")
		}
		targets = sortedTargetsFromManifest(e, manifest)
	}

	printTargets(ctx, "Updating Fly skills in:", targets)

	return syncSkills(ctx, e, targets, "updated", flag.GetBool(ctx, "force"), false)
}
