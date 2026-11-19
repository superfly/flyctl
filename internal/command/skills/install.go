package skills

import (
	"context"
	"errors"
	"fmt"
	"io"
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
			Description: AgentFlagDescription(),
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

	return syncSkills(ctx, e, targets, "installed", flag.GetBool(ctx, "force"), false)
}

// PreservedFile describes a local change left untouched during installation.
type PreservedFile struct {
	Path   string `json:"path"` // relative to the target's skills directory
	Reason string `json:"reason"`
}

// InstallResult is the outcome for one target. Incomplete means an incoming
// file is still missing because its local deletion was preserved.
type InstallResult struct {
	Err        error
	Preserved  []PreservedFile
	Incomplete bool
}

// InstallInto downloads the Fly skills once and writes them into targets.
// Results are keyed by Target.Key. A target failure does not stop the others;
// the returned error is reserved for shared download or manifest failures.
// verb names a fresh write in the output ("installed"). When quiet is set,
// per-skill lines, the closing summary and the progress indicator are omitted;
// the files and the manifest are still written. fly setup agent uses quiet
// for --json and for the installer's embedded output.
func InstallInto(ctx context.Context, targets []Target, verb string, force, quiet bool) (map[string]InstallResult, error) {
	e, err := envFromContext(ctx)
	if err != nil {
		return nil, err
	}

	return syncSkillsWithResults(ctx, e, targets, verb, force, quiet, "fly skills install --force")
}

// syncSkills downloads the skills and applies them to every target, then
// records the result in the manifest. verb names a fresh write in output.
// quiet drops the per-skill lines and the closing summary.
func syncSkills(ctx context.Context, e env, targets []Target, verb string, force, quiet bool) error {
	results, err := syncSkillsWithResults(ctx, e, targets, verb, force, quiet, "--force")
	for _, target := range targets {
		err = errors.Join(err, results[target.Key()].Err)
	}

	return err
}

func syncSkillsWithResults(ctx context.Context, e env, targets []Target, verb string, force, quiet bool, forceHint string) (map[string]InstallResult, error) {
	streams := iostreams.FromContext(ctx)
	out := streams.Out
	if quiet {
		out = io.Discard
		savedErr := streams.ErrOut
		streams.ErrOut = io.Discard
		defer func() { streams.ErrOut = savedErr }()
	}
	colorize := streams.ColorScheme()

	manifest, err := readManifest(e.configDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", manifestPath(e.configDir), err)
	}

	src := sourceFromFlags(ctx, manifest)
	dl, err := fetchSkills(ctx, src)
	if err != nil {
		return nil, err
	}
	sha, skills := dl.SHA, dl.Skills

	var (
		changed int
		skipped int
		failed  int
	)
	results := make(map[string]InstallResult, len(targets))

	for _, t := range targets {
		key := t.Key()
		var result InstallResult
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
			for _, rel := range res.skippedFiles() {
				result.Preserved = append(result.Preserved, PreservedFile{Path: name + "/" + rel, Reason: res.Skipped[rel]})
				if res.Skipped[rel] == "deleted locally" {
					result.Incomplete = true
				}
			}
			if err != nil {
				failed++
				result.Err = errors.Join(result.Err, err)
				// applySkill may have written files before a later file failed.
				// Keep those receipts as well as all previously tracked files.
				if hasRec || len(res.Written)+len(res.Adopted)+len(res.Removed) > 0 {
					manifest.setRecord(key, name, partialRecord(rec, files, res))
				}
				fmt.Fprintf(out, "%s %s: %s failed: %v\n", colorize.FailureIcon(), t.ToolName, name, err)

				continue
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
				fmt.Fprintf(out, "%s %s: removed retired skill %s %s\n", colorize.SuccessIcon(), colorize.Bold(t.ToolName), colorize.Green(name), colorize.Gray(skillDir))
			case res.changed() && state == NotInstalled:
				changed++
				fmt.Fprintf(out, "%s %s: %s %s %s\n", colorize.SuccessIcon(), colorize.Bold(t.ToolName), verb, colorize.Green(name), colorize.Gray(skillDir))
			case res.changed():
				changed++
				label := "updated"
				if force && (state == Modified || state == Unverifiable) {
					label = "overwrote"
				}
				fmt.Fprintf(out, "%s %s: %s %s %s\n", colorize.SuccessIcon(), colorize.Bold(t.ToolName), label, colorize.Green(name), colorize.Gray(skillDir))
			case len(res.Skipped) == 0:
				fmt.Fprintf(out, "%s %s: %s already up to date\n", colorize.Gray("-"), t.ToolName, name)
			}

			if files := res.skippedFiles(); len(files) > 0 {
				skipped++
				fmt.Fprintf(out, "%s %s: kept your local copy of %s\n", colorize.WarningIcon(), colorize.Bold(t.ToolName), colorize.Yellow(name))
				for _, rel := range files {
					fmt.Fprintf(out, "    %s (%s)\n", joinRel(skillDir, rel), res.Skipped[rel])
				}
			}
		}
		results[key] = result
	}

	// The top-level revision and archive hash describe the last download that
	// was applied in full; a run that kept local edits leaves them alone.
	previous := manifest.SourceSHA
	if skipped == 0 && failed == 0 {
		manifest.SourceSHA = sha
		manifest.ArchiveSHA256 = dl.ArchiveSHA256
	}
	if sha != "" {
		manifest.LatestSHA = sha
		manifest.LatestRef = src.refOrDefault()
		manifest.LastChecked = nowRFC3339()
	}
	// The pin records the ref this run asked for, applied in full or not, so
	// the next update and status follow it. Main clears it.
	manifest.SourceRef = src.ref
	if err := manifest.save(e.configDir); err != nil {
		return results, fmt.Errorf("failed to write %s: %w", manifestPath(e.configDir), err)
	}

	fmt.Fprintln(out)
	switch {
	case failed > 0:
		fmt.Fprintf(out, "%s %d skill(s) %s; %d failed; %d kept local changes. Completed writes were recorded.\n",
			colorize.FailureIcon(), changed, verb, failed, skipped)
	case changed > 0 && skipped > 0:
		fmt.Fprintf(out, "%s %d skill(s) %s; %d skipped because of local changes. To overwrite them, use %s.\n",
			colorize.Yellow("!"), changed, verb, skipped, colorize.Cyan(forceHint))
	case changed > 0:
		fmt.Fprintf(out, "%s Fly skills %s at %s.\n", colorize.SuccessIcon(), verb, shortSHA(sha))
		fmt.Fprintf(out, "%s You may need to restart your coding agent(s) to load them.\n", colorize.Yellow("!"))
	case skipped > 0:
		fmt.Fprintf(out, "%s No skills were %s: every pending change touched a file you modified or deleted. To overwrite them, use %s.\n",
			colorize.Yellow("!"), verb, colorize.Cyan(forceHint))
	default:
		fmt.Fprintf(out, "%s Fly skills already up to date at %s.\n", colorize.SuccessIcon(), shortSHA(sha))
	}
	if previous != "" && sha != "" && previous != sha && skipped == 0 && failed == 0 {
		fmt.Fprintf(out, "  revision %s → %s\n", shortSHA(previous), shortSHA(sha))
	}
	if !quiet {
		printPin(ctx, src.ref)
	}

	return results, nil
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
