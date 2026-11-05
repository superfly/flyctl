package skills

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
)

func newStatus() *cobra.Command {
	const (
		short = "Show the installed Fly agent skills and whether an update is available"
		long  = `Show, for every coding agent the Fly skills are installed into, the revision
installed and any files modified since, and whether a newer revision is
available upstream.

An install pinned with --ref is compared against that branch, tag or commit.
Pass --ref here to compare against another one without changing the pin.`
	)

	cmd := command.New("status", short, long, runStatus)
	cmd.Args = cobra.NoArgs
	addStatusFlags(cmd)

	return cmd
}

func addStatusFlags(cmd *cobra.Command) {
	flag.Add(cmd,
		flag.JSONOutput(),
		flag.Bool{
			Name:        "offline",
			Description: "Do not contact GitHub to check for a newer revision",
		},
	)
	addSourceFlags(cmd, statusRefDescription)
}

// statusSkill is one skill's status in one target.
type statusSkill struct {
	Name        string   `json:"name"`
	SourceSHA   string   `json:"source_sha,omitempty"`
	InstalledAt string   `json:"installed_at"`
	Modified    []string `json:"modified_files,omitempty"`
}

// statusTarget is one target's status.
type statusTarget struct {
	Tool      string        `json:"tool"`
	SkillsDir string        `json:"skills_dir"`
	SourceSHA string        `json:"source_sha,omitempty"`
	Mixed     bool          `json:"mixed_revisions,omitempty"`
	Skills    []statusSkill `json:"skills"`
}

type statusReport struct {
	Installed bool   `json:"installed"`
	SourceSHA string `json:"source_sha,omitempty"`
	// ArchiveSHA256 is the hex digest of the archive the installed files
	// came from.
	ArchiveSHA256 string `json:"archive_sha256,omitempty"`
	// SourceRef is the pin recorded by install or update; empty tracks main.
	SourceRef string `json:"source_ref,omitempty"`
	// CheckedRef is the branch, tag or commit LatestSHA is the head of.
	CheckedRef      string         `json:"checked_ref"`
	LatestSHA       string         `json:"latest_sha,omitempty"`
	UpdateAvailable bool           `json:"update_available"`
	Targets         []statusTarget `json:"targets"`
	Unmanaged       []statusTarget `json:"unmanaged,omitempty"`
}

func runStatus(ctx context.Context) error {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	cfg := config.FromContext(ctx)

	e, err := envFromContext(ctx)
	if err != nil {
		return err
	}

	manifest, err := readManifest(e.configDir)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", manifestPath(e.configDir), err)
	}

	// Status reads the pin but never writes it: --ref here only changes
	// what this run compares against.
	src := sourceFromFlags(ctx, manifest)

	report := statusReport{
		Installed:     manifest.hasInstalledSkills(),
		SourceSHA:     manifest.SourceSHA,
		ArchiveSHA256: manifest.ArchiveSHA256,
		SourceRef:     manifest.SourceRef,
		CheckedRef:    src.refOrDefault(),
	}
	// Never label a cached revision as belonging to another ref, including
	// when offline or when the lookup below fails. Legacy caches have no ref.
	if manifest.LatestRef == report.CheckedRef {
		report.LatestSHA = manifest.LatestSHA
	}

	for _, t := range sortedTargetsFromManifest(e, manifest) {
		key := t.Key()
		sha, mixed := manifest.targetRevision(key)
		st := statusTarget{Tool: t.ToolName, SkillsDir: t.SkillsDir, SourceSHA: sha, Mixed: mixed}
		for _, name := range sortedKeys(manifest.Targets[key]) {
			if !validSkillName(name) {
				continue
			}
			rec := manifest.Targets[key][name]
			st.Skills = append(st.Skills, statusSkill{
				Name:        name,
				SourceSHA:   rec.SourceSHA,
				InstalledAt: rec.InstalledAt,
				Modified:    modifiedFiles(joinRel(t.SkillsDir, name), rec),
			})
		}
		report.Targets = append(report.Targets, st)
	}

	// Detected agents with a Fly skill on disk that the manifest does not
	// track: installs from before flyctl managed them, or copies synced by
	// hand.
	for _, tool := range codingTools(e.home) {
		target := Target{Slug: tool.Slug, ToolName: tool.Name, SkillsDir: tool.SkillsDir()}
		if len(manifest.Targets[target.Key()]) > 0 {
			continue
		}
		if _, ok := hashDiskFile(joinRel(target.SkillsDir, "fly/SKILL.md")); ok {
			report.Unmanaged = append(report.Unmanaged, statusTarget{Tool: tool.Name, SkillsDir: target.SkillsDir})
		}
	}

	// A ref that is already a commit SHA needs no lookup, so it is honoured
	// even offline.
	if report.Installed && (src.pinned() || !flag.GetBool(ctx, "offline")) {
		io.StartProgressIndicatorMsg("Checking for the latest Fly skills")
		latest, err := fetchLatestSHA(ctx, src)
		io.StopProgressIndicator()
		if err != nil {
			fmt.Fprintf(io.ErrOut, "%s Could not check upstream: %v\n", colorize.WarningIcon(), err)
		} else if latest != "" {
			report.LatestSHA = latest
			manifest.LatestSHA = latest
			manifest.LatestRef = report.CheckedRef
			manifest.LastChecked = nowRFC3339()
			if err := manifest.save(e.configDir); err != nil {
				fmt.Fprintf(io.ErrOut, "%s Could not record the upstream check: %v\n", colorize.WarningIcon(), err)
			}
		}
	}
	report.UpdateAvailable = report.LatestSHA != "" && report.Installed && anyTargetBehind(report)

	if cfg.JSONOutput {
		return json.NewEncoder(io.Out).Encode(report)
	}

	if !report.Installed {
		fmt.Fprintf(io.Out, "Fly skills are not installed. Run %s to install them.\n", colorize.Cyan("fly skills install"))
		printUnmanaged(ctx, report.Unmanaged)
		return nil
	}

	fmt.Fprintf(io.Out, "%s\n", colorize.Bold("Fly skills"))
	for _, t := range report.Targets {
		rev := "unknown revision"
		switch {
		case t.Mixed:
			rev = "mixed revisions"
		case t.SourceSHA != "":
			rev = shortSHA(t.SourceSHA)
		}
		fmt.Fprintf(io.Out, "  %s %s %s\n", colorize.Bold(t.Tool), rev, colorize.Gray(t.SkillsDir))
		for _, s := range t.Skills {
			line := fmt.Sprintf("    %s", s.Name)
			if t.Mixed && s.SourceSHA != "" {
				line += " " + shortSHA(s.SourceSHA)
			}
			if len(s.Modified) > 0 {
				line += " " + colorize.Yellow("modified: "+joinOrNone(s.Modified))
			}
			fmt.Fprintln(io.Out, line)
		}
	}
	printUnmanaged(ctx, report.Unmanaged)

	fmt.Fprintln(io.Out)
	if report.ArchiveSHA256 != "" {
		fmt.Fprintf(io.Out, "Archive sha256 %s\n", colorize.Gray(report.ArchiveSHA256))
	}
	printPin(ctx, report.SourceRef)
	upstream := "upstream"
	switch {
	case src.pinned():
		upstream = "the pinned commit"
	case src.ref != "":
		upstream = src.ref
	}
	switch {
	case report.LatestSHA == "":
		fmt.Fprintf(io.Out, "Revision of %s unknown.\n", upstream)
	case report.UpdateAvailable:
		fmt.Fprintf(io.Out, "%s A newer revision is available on %s (%s). Run %s.\n", colorize.Yellow("!"), upstream, shortSHA(report.LatestSHA), colorize.Cyan("fly skills update"))
	default:
		fmt.Fprintf(io.Out, "%s Up to date with %s (%s).\n", colorize.SuccessIcon(), upstream, shortSHA(report.LatestSHA))
	}

	return nil
}

func anyTargetBehind(report statusReport) bool {
	for _, t := range report.Targets {
		if t.Mixed || t.SourceSHA != report.LatestSHA {
			return true
		}
	}

	return false
}

func printUnmanaged(ctx context.Context, unmanaged []statusTarget) {
	if len(unmanaged) == 0 {
		return
	}
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "\n%s Fly skills found that flyctl does not manage:\n", colorize.WarningIcon())
	for _, t := range unmanaged {
		fmt.Fprintf(io.Out, "  %s %s\n", t.Tool, colorize.Gray(t.SkillsDir))
	}
	fmt.Fprintf(io.Out, "  Run %s to adopt unmodified copies.\n", colorize.Cyan("fly skills install"))
}
