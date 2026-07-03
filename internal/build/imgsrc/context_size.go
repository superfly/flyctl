package imgsrc

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/docker/go-units"
	humanize "github.com/dustin/go-humanize"
	"github.com/moby/patternmatcher"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

// defaultBuildContextWarnSize is the build context size above which flyctl warns
// that the context being uploaded to the builder is unexpectedly large. It can
// be overridden per-build with the --build-context-warn-size flag or the
// FLY_BUILD_CONTEXT_WARN_SIZE environment variable (set either to 0 to disable
// the warning). Values accept a plain number (in MB) or a human-readable size
// such as "512mb" or "1gb".
const defaultBuildContextWarnSize = "100mb"

const buildContextWarnEnvVar = "FLY_BUILD_CONTEXT_WARN_SIZE"

// maxLargeContextPaths is the number of largest paths listed in the warning.
const maxLargeContextPaths = 5

// minLargeContextPathBytes is the smallest a path can be and still be worth
// listing as an offender in the warning.
const minLargeContextPathBytes = 1_000_000

type buildContextEntry struct {
	name  string
	bytes int64
	isDir bool
}

func (e buildContextEntry) displayName() string {
	if e.isDir {
		return e.name + "/"
	}

	return e.name
}

type buildContextStats struct {
	totalBytes int64
	fileCount  int
	// entries holds the top-level files and directories of the build context,
	// each carrying the combined size of the (non-ignored) files beneath it,
	// sorted largest first.
	entries []buildContextEntry
}

// warnOnLargeBuildContext prints a warning when the Docker build context that
// will be uploaded to the builder is larger than the configured threshold,
// listing the largest paths so the user can add anything unnecessary to their
// .dockerignore. flagValue is the value of the --build-context-warn-size flag,
// or "" when it was not set. It is best-effort: any error simply skips the
// warning, so a deploy is never blocked or slowed meaningfully by it.
func warnOnLargeBuildContext(streams *iostreams.IOStreams, opts ImageOptions, flagValue string) {
	// Only plain Dockerfile builds upload a .dockerignore-filtered context.
	// Buildpacks, builtins and custom builders manage their own context, so the
	// estimate below wouldn't reflect what they send.
	if opts.Builder != "" || opts.BuiltIn != "" || len(opts.Buildpacks) > 0 {
		return
	}
	if opts.WorkingDir == "" {
		return
	}

	thresholdBytes, enabled := resolveBuildContextWarnBytes(flagValue, env.First(buildContextWarnEnvVar))
	if !enabled {
		return
	}

	dockerfile := opts.DockerfilePath
	if dockerfile == "" {
		dockerfile = ResolveDockerfile(opts.WorkingDir)
	}
	if dockerfile == "" {
		return
	}

	stats, err := analyzeBuildContext(opts.WorkingDir, dockerfile, opts.IgnorefilePath)
	if err != nil {
		terminal.Debugf("skipping build context size check: %v\n", err)

		return
	}

	if stats.totalBytes < thresholdBytes {
		return
	}

	fmt.Fprint(streams.ErrOut, formatLargeBuildContextWarning(streams.ColorScheme(), stats))
}

// resolveBuildContextWarnBytes turns the flag value (or "" when
// --build-context-warn-size was not set) and the environment variable value into
// a threshold in bytes. An explicit flag takes precedence over the env var,
// which takes precedence over the default. Each accepts a plain number (in MB)
// or a human-readable size such as "512mb" or "1gb". The boolean is false when
// the warning has been disabled (threshold resolved to 0 or below).
func resolveBuildContextWarnBytes(flagValue, envValue string) (int64, bool) {
	raw := defaultBuildContextWarnSize
	if envValue != "" {
		raw = envValue
	}
	if flagValue != "" {
		raw = flagValue
	}

	mb, err := helpers.ParseSize(raw, units.RAMInBytes, units.MiB)
	if err != nil {
		terminal.Debugf("ignoring invalid build context warn size %q: %v\n", raw, err)
		mb, _ = helpers.ParseSize(defaultBuildContextWarnSize, units.RAMInBytes, units.MiB)
	}

	if mb <= 0 {
		return 0, false
	}

	return int64(mb) * units.MiB, true
}

// analyzeBuildContext walks workingDir applying the same .dockerignore rules
// flyctl uses when building the image, and returns the total size of the
// resulting build context along with its largest top-level paths. It only
// stats files (never reads them), and prunes ignored directories, so it stays
// cheap relative to the upload it is warning about.
func analyzeBuildContext(workingDir, dockerfile, ignoreFile string) (buildContextStats, error) {
	var stats buildContextStats

	relDockerfile := ""
	if isPathInRoot(dockerfile, workingDir) {
		if p, err := filepath.Rel(workingDir, dockerfile); err == nil {
			relDockerfile = filepath.ToSlash(p)
		}
	}

	excludes, err := readDockerignore(workingDir, ignoreFile, relDockerfile)
	if err != nil {
		return stats, err
	}

	pm, err := patternmatcher.New(excludes)
	if err != nil {
		return stats, err
	}

	byTopLevel := map[string]*buildContextEntry{}

	walkErr := filepath.WalkDir(workingDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Skip entries we can't read rather than aborting the whole estimate.
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}

			return nil
		}

		rel, relErr := filepath.Rel(workingDir, path)
		if relErr != nil || rel == "." {
			return nil
		}
		slashRel := filepath.ToSlash(rel)

		// Match the file against .dockerignore exactly like the archiver does,
		// taking parent directory matches into account.
		if excluded, _ := pm.MatchesOrParentMatches(slashRel); excluded {
			// A directory may be skipped wholesale unless a negated pattern
			// (e.g. !build/keep) could re-include something beneath it.
			if d.IsDir() && canSkipExcludedDir(pm, rel) {
				return fs.SkipDir
			}

			return nil
		}

		top := slashRel
		if i := strings.IndexByte(slashRel, '/'); i >= 0 {
			top = slashRel[:i]
		}
		entry := byTopLevel[top]
		if entry == nil {
			entry = &buildContextEntry{name: top}
			byTopLevel[top] = entry
		}
		if slashRel != top || d.IsDir() {
			entry.isDir = true
		}

		if d.Type().IsRegular() {
			info, infoErr := d.Info()
			if infoErr != nil {
				return nil
			}
			entry.bytes += info.Size()
			stats.totalBytes += info.Size()
			stats.fileCount++
		}

		return nil
	})
	if walkErr != nil {
		return stats, walkErr
	}

	stats.entries = make([]buildContextEntry, 0, len(byTopLevel))
	for _, e := range byTopLevel {
		stats.entries = append(stats.entries, *e)
	}
	sort.Slice(stats.entries, func(i, j int) bool {
		if stats.entries[i].bytes != stats.entries[j].bytes {
			return stats.entries[i].bytes > stats.entries[j].bytes
		}

		return stats.entries[i].name < stats.entries[j].name
	})

	return stats, nil
}

// canSkipExcludedDir reports whether an excluded directory can be skipped
// entirely, i.e. no negated (re-include) pattern could match a file beneath it.
// This mirrors the directory-skipping logic in
// github.com/docker/docker/pkg/archive so our estimate matches what is sent.
func canSkipExcludedDir(pm *patternmatcher.PatternMatcher, relDir string) bool {
	if !pm.Exclusions() {
		return true
	}

	dirSlash := relDir + string(filepath.Separator)
	for _, pat := range pm.Patterns() {
		if !pat.Exclusion() {
			continue
		}
		if strings.HasPrefix(pat.String()+string(filepath.Separator), dirSlash) {
			return false
		}
	}

	return true
}

// largeContextPaths returns the entries worth surfacing as likely offenders:
// the largest few that are individually big enough to matter.
func largeContextPaths(stats buildContextStats) []buildContextEntry {
	out := make([]buildContextEntry, 0, maxLargeContextPaths)
	for _, e := range stats.entries {
		if e.bytes < minLargeContextPathBytes {
			break // entries are sorted largest first
		}
		out = append(out, e)
		if len(out) == maxLargeContextPaths {
			break
		}
	}

	return out
}

func formatLargeBuildContextWarning(cs *iostreams.ColorScheme, stats buildContextStats) string {
	fileWord := "files"
	if stats.fileCount == 1 {
		fileWord = "file"
	}

	var b strings.Builder
	b.WriteByte('\n')
	fmt.Fprintf(&b, "%s Build context is %s across %s %s. It is uploaded to the builder on\n",
		cs.Yellow("WARN"),
		humanize.Bytes(uint64(stats.totalBytes)),
		humanize.Comma(int64(stats.fileCount)),
		fileWord,
	)
	fmt.Fprint(&b, "     every deploy, and a large context can make builds noticeably slower.\n")

	if shown := largeContextPaths(stats); len(shown) > 0 {
		fmt.Fprint(&b, "     Largest paths in the context:\n")
		nameWidth := 0
		for _, e := range shown {
			if n := len(e.displayName()); n > nameWidth {
				nameWidth = n
			}
		}
		for _, e := range shown {
			fmt.Fprintf(&b, "       %-*s  %s\n", nameWidth, e.displayName(), humanize.Bytes(uint64(e.bytes)))
		}
	}

	fmt.Fprint(&b, "     If some of these don't need to be in the image, add them to .dockerignore:\n")
	fmt.Fprintf(&b, "     %s\n", cs.Gray("https://docs.docker.com/build/concepts/context/#dockerignore-files"))
	fmt.Fprintf(&b, "     %s\n", cs.Gray(fmt.Sprintf("(disable with --%s 0)", flag.BuildContextWarnSizeName)))

	return b.String()
}
