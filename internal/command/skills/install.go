package skills

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

func newInstall() *cobra.Command {
	const (
		short = "Install the Fly agent skills into local coding agents"
		long  = `Download the Fly agent skills from github.com/superfly/skills and install
them into ~/.agents/skills and the global skills directory of every detected
coding agent (Claude Code, Codex, Cursor, OpenCode, GitHub Copilot, Factory
Droid, Pi and Antigravity). With --agent, exactly the agents you name are
written, so include universal to write ~/.agents/skills.

Files you have edited since the last install are left alone; pass --force to
overwrite them.

Skills are fetched from github.com/superfly/skills over TLS: main is resolved
to a commit SHA through the GitHub API, the archive for that exact SHA is
downloaded, and the SHA, the archive's SHA-256 and a hash of every file written
are recorded in ~/.fly/skills.json. There is no separate signature, so trust
rests on TLS to github.com and on write access to the superfly/skills
repository, the same model as flyctl's own install script. Pass --ref with a
tag or commit SHA to pin a revision instead of tracking main; the pin is
remembered by later update and status runs until you pass --ref main.`
	)

	cmd := command.New("install", short, long, runInstall)
	cmd.Args = cobra.NoArgs
	cmd.Aliases = []string{"add"}

	flag.Add(cmd,
		flag.StringSlice{
			Name:        "agent",
			Description: agentFlagDescription,
		},
		flag.Bool{
			Name:        "force",
			Description: "Overwrite files you have modified locally instead of skipping them",
		},
		flag.Yes(),
	)
	addSourceFlags(cmd, installRefDescription)

	return cmd
}

func runInstall(ctx context.Context) error {
	e, err := envFromContext(ctx)
	if err != nil {
		return err
	}

	tools, err := resolveTools(e.home, flag.GetStringSlice(ctx, "agent"))
	if err != nil {
		return err
	}
	targets := buildTargets(tools)

	printTargets(ctx, "Installing Fly skills to:", targets)

	if !flag.GetYes(ctx) {
		confirmed, err := prompt.ConfirmYes(ctx, fmt.Sprintf("Install into %d director%s?", len(targets), plural(len(targets), "y", "ies")))
		if err != nil && !prompt.IsNonInteractive(err) {
			return err
		}
		if err == nil && !confirmed {
			return errors.New("install cancelled")
		}
	}

	return syncSkills(ctx, e, targets, "installed", flag.GetBool(ctx, "force"))
}

// syncSkills downloads the skills and applies them to every target, then
// records the result in the manifest. verb names a fresh write in output.
func syncSkills(ctx context.Context, e env, targets []Target, verb string, force bool) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	manifest, err := readManifest(e.configDir)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", manifestPath(e.configDir), err)
	}

	src := sourceFromFlags(ctx, manifest)
	dl, err := fetchSkills(ctx, src)
	if err != nil {
		return err
	}
	sha, skills := dl.SHA, dl.Skills

	var (
		changed int
		skipped int
	)

	for _, t := range targets {
		key := t.Key()
		names := map[string]bool{}
		for name := range skills {
			names[name] = true
		}
		for name := range manifest.Targets[key] {
			names[name] = true
		}
		for _, name := range sortedKeys(names) {
			files, incoming := skills[name]
			skillDir := joinRel(t.SkillsDir, name)
			rec, hasRec := manifest.record(key, name)
			state := classifySkill(skillDir, newFileHashes(files), rec, hasRec)

			next, res, err := applySkill(skillDir, files, rec, hasRec, sha, force)
			if err != nil {
				return err
			}
			if !incoming && len(res.Skipped) == 0 {
				manifest.deleteRecord(key, name)
				// Only remove an empty directory; user-added files survive.
				_ = os.Remove(skillDir)
			} else {
				manifest.setRecord(key, name, next)
			}

			switch {
			case !incoming && len(res.Skipped) == 0:
				changed++
				fmt.Fprintf(io.Out, "%s %s: removed retired skill %s %s\n", colorize.SuccessIcon(), colorize.Bold(t.ToolName), colorize.Green(name), colorize.Gray(skillDir))
			case res.changed() && state == NotInstalled:
				changed++
				fmt.Fprintf(io.Out, "%s %s: %s %s %s\n", colorize.SuccessIcon(), colorize.Bold(t.ToolName), verb, colorize.Green(name), colorize.Gray(skillDir))
			case res.changed():
				changed++
				label := "updated"
				if force && (state == Modified || state == Unverifiable) {
					label = "overwrote"
				}
				fmt.Fprintf(io.Out, "%s %s: %s %s %s\n", colorize.SuccessIcon(), colorize.Bold(t.ToolName), label, colorize.Green(name), colorize.Gray(skillDir))
			case len(res.Skipped) == 0:
				fmt.Fprintf(io.Out, "%s %s: %s already up to date\n", colorize.Gray("-"), t.ToolName, name)
			}

			if files := res.skippedFiles(); len(files) > 0 {
				skipped++
				fmt.Fprintf(io.Out, "%s %s: kept your local copy of %s\n", colorize.WarningIcon(), colorize.Bold(t.ToolName), colorize.Yellow(name))
				for _, rel := range files {
					fmt.Fprintf(io.Out, "    %s (%s)\n", joinRel(skillDir, rel), res.Skipped[rel])
				}
			}
		}
	}

	// The top-level revision and archive hash describe the last download that
	// was applied in full; a run that kept local edits leaves them alone.
	previous := manifest.SourceSHA
	if skipped == 0 {
		manifest.SourceSHA = sha
		manifest.ArchiveSHA256 = dl.ArchiveSHA256
	}
	if sha != "" {
		manifest.LatestSHA = sha
		manifest.LastChecked = nowRFC3339()
	}
	// The pin records the ref this run asked for, applied in full or not, so
	// the next update and status follow it. Main clears it.
	manifest.SourceRef = src.ref
	if err := manifest.save(e.configDir); err != nil {
		return fmt.Errorf("failed to write %s: %w", manifestPath(e.configDir), err)
	}

	fmt.Fprintln(io.Out)
	switch {
	case changed > 0 && skipped > 0:
		fmt.Fprintf(io.Out, "%s %d skill(s) %s; %d skipped because of local changes. Re-run with %s to overwrite them.\n",
			colorize.Yellow("!"), changed, verb, skipped, colorize.Cyan("--force"))
	case changed > 0:
		fmt.Fprintf(io.Out, "%s Fly skills %s at %s.\n", colorize.SuccessIcon(), verb, shortSHA(sha))
		fmt.Fprintf(io.Out, "%s You may need to restart your coding agent(s) to load them.\n", colorize.Yellow("!"))
	case skipped > 0:
		fmt.Fprintf(io.Out, "%s No skills were %s: every pending change touched a file you modified. Re-run with %s to overwrite them.\n",
			colorize.Yellow("!"), verb, colorize.Cyan("--force"))
	default:
		fmt.Fprintf(io.Out, "%s Fly skills already up to date at %s.\n", colorize.SuccessIcon(), shortSHA(sha))
	}
	if previous != "" && sha != "" && previous != sha && skipped == 0 {
		fmt.Fprintf(io.Out, "  revision %s → %s\n", shortSHA(previous), shortSHA(sha))
	}
	printPin(ctx, src.ref)

	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// sortedTargetsFromManifest rebuilds targets from the manifest's keys so
// update and remove act on what was actually installed.
func sortedTargetsFromManifest(e env, manifest *Manifest) []Target {
	keys := sortedKeys(manifest.Targets)
	targets := make([]Target, 0, len(keys))
	for _, key := range keys {
		targets = append(targets, Target{
			ToolName:  toolNameForKey(e.home, key),
			SkillsDir: joinRel("", key),
		})
	}
	sort.SliceStable(targets, func(i, j int) bool {
		// Universal first, mirroring install order.
		return targets[i].ToolName == "Universal (.agents)" && targets[j].ToolName != "Universal (.agents)"
	})

	return targets
}
