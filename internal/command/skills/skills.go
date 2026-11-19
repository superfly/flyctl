// Package skills implements the skills command chain, which installs the Fly
// agent skills from github.com/superfly/skills into local coding agents.
package skills

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
)

// AgentFlagDescription is the shared --agent help text. The slug list comes
// from codingTools so fly skills and fly setup agent cannot drift apart.
func AgentFlagDescription() string {
	return "Act on exactly the named agent(s) instead of detecting them; " +
		"include universal to write ~/.agents/skills. " +
		"One of: " + strings.Join(KnownAgentSlugs(), ", ") + ". " +
		"Can be repeated or comma-separated."
}

// New initializes and returns a new skills Command.
func New() *cobra.Command {
	const (
		short = "Manage the Fly agent skills installed into local coding agents"
		long  = `Install, update and remove the Fly agent skills from github.com/superfly/skills.

Skills are installed into ~/.agents/skills and into the global skills
directory of every detected coding agent: Claude Code, Codex, Cursor,
OpenCode, GitHub Copilot, Factory Droid, Pi and Antigravity. With --agent,
exactly the agents you name are written, so include universal to write
~/.agents/skills.

Without a subcommand this shows the installed revision per agent and whether
a newer revision is available upstream.`
	)

	cmd := command.New("skills", short, long, runStatus)
	cmd.Args = cobra.NoArgs

	addStatusFlags(cmd)

	cmd.AddCommand(
		newInstall(),
		newUpdate(),
		newRemove(),
		newStatus(),
	)

	return cmd
}

const (
	installRefDescription = "Install from a branch, tag or commit SHA of " + skillsRepo + ". " +
		"The pin is remembered by later update and status runs; pass --ref " + skillsRef + " to track " + skillsRef + " again."
	statusRefDescription = "Compare against a branch, tag or commit SHA of " + skillsRepo + " " +
		"instead of the remembered pin or " + skillsRef + ". Does not change the pin."
)

// addSourceFlags adds the flags that point a run at a different branch,
// commit or tarball, with refDescription explaining --ref for that command.
// --tarball stays hidden. The FLY_SKILLS_REF and FLY_SKILLS_TARBALL
// environment variables do the same for scripted use.
func addSourceFlags(cmd *cobra.Command, refDescription string) {
	flag.Add(cmd,
		flag.String{
			Name:        "ref",
			Description: refDescription,
		},
		flag.String{
			Name:        "tarball",
			Description: "URL or local path of a skills tarball to install from instead of downloading",
			Hidden:      true,
		},
	)
}

// resolveRef picks the branch, tag or commit to act on. An explicit --ref
// beats FLY_SKILLS_REF, which beats the pin recorded in the manifest, which
// beats main. The result is "" for main, so naming main clears a pin.
func resolveRef(ctx context.Context, manifest *Manifest) string {
	switch {
	case flag.IsSpecified(ctx, "ref"):
		return normalizeRef(flag.GetString(ctx, "ref"))
	case os.Getenv(envRef) != "":
		return normalizeRef(os.Getenv(envRef))
	case manifest != nil:
		return normalizeRef(manifest.SourceRef)
	}

	return ""
}

func normalizeRef(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == skillsRef {
		return ""
	}

	return ref
}

// sourceFromFlags builds the download source for this run from the flags,
// the environment and the manifest's recorded pin.
func sourceFromFlags(ctx context.Context, manifest *Manifest) source {
	src := sourceFromEnv()
	src.ref = resolveRef(ctx, manifest)
	if tarball := flag.GetString(ctx, "tarball"); tarball != "" {
		src.tarball = tarball
	}

	return src
}

// printPin says which ref is in effect when it is not main, and how to go
// back to tracking main.
func printPin(ctx context.Context, ref string) {
	if ref == "" {
		return
	}
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()
	fmt.Fprintf(io.Out, "Pinned to %s (pass %s to track %s).\n", colorize.Cyan(ref), colorize.Cyan("--ref "+skillsRef), skillsRef)
}

// env is everything the subcommands need to know about where things live.
type env struct {
	home      string
	configDir string
}

func envFromContext(ctx context.Context) (env, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return env{}, fmt.Errorf("could not determine home directory: %w", err)
	}
	return env{home: home, configDir: state.ConfigDirectory(ctx)}, nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// download is a resolved and verified skills download.
type download struct {
	// SHA is the skills repository commit, or "" for a local tarball.
	SHA string
	// ArchiveSHA256 is the hex digest of the tarball bytes.
	ArchiveSHA256 string
	Skills        SkillFiles
}

// fetchSkills resolves the upstream revision, downloads the archive pinned to
// it, checks the archive is for that revision, and extracts the skills,
// reporting progress on stderr.
func fetchSkills(ctx context.Context, src source) (*download, error) {
	io := iostreams.FromContext(ctx)

	io.StartProgressIndicatorMsg("Checking for the latest Fly skills")
	sha, err := fetchLatestSHA(ctx, src)
	if err != nil {
		io.StopProgressIndicator()
		return nil, err
	}

	io.ChangeProgressIndicatorMsg("Downloading Fly skills")
	tarball, err := downloadTarball(ctx, src, sha)
	io.StopProgressIndicator()
	if err != nil {
		return nil, err
	}

	a, err := extractArchive(tarball, skillsPathPrefix)
	if err != nil {
		return nil, err
	}
	if err := verifyArchiveRevision(src, sha, a.TopLevel); err != nil {
		return nil, err
	}

	return &download{SHA: sha, ArchiveSHA256: a.SHA256, Skills: a.Skills}, nil
}

func printTargets(ctx context.Context, action string, targets []Target) {
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	fmt.Fprintf(io.Out, "%s\n", colorize.Bold(action))
	for _, t := range targets {
		fmt.Fprintf(io.Out, "  %s %s\n", t.ToolName, colorize.Gray(t.SkillsDir))
	}
	fmt.Fprintln(io.Out)
}

func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "none"
	}
	return strings.Join(items, ", ")
}
