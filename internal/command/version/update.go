package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/blang/semver"
	"github.com/samber/lo"
	"github.com/spf13/cobra"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/iostreams"
)

func newUpdate() *cobra.Command {
	const (
		short = "Checks for available updates and automatically updates"

		long = `Checks for update and if one is available, runs the appropriate
command to update the application.`
	)

	return command.New("update", short, long, runUpdate)
}

func runUpdate(ctx context.Context) error {
	release, err := update.LatestRelease(ctx, cache.FromContext(ctx).Channel())
	switch {
	case err != nil:
		return fmt.Errorf("failed determining latest release: %w", err)
	case release == nil:
		return fmt.Errorf("failed querying latest release information: %w", err)
	}

	latest, err := semver.ParseTolerant(release.Version)
	if err != nil {
		return fmt.Errorf("error parsing latest release version number %q: %w",
			release.Version, err)
	}

	if buildinfo.Version().GTE(latest) {
		return errors.New("no available update")
	}

	io := iostreams.FromContext(ctx)

	if err = update.UpgradeInPlace(ctx, io, release.Prerelease); err != nil {
		return err
	}

	return printVersionUpdate(ctx, buildinfo.Version())
}

// printVersionUpdate prints "Updated flyctl [oldVersion] -> [newVersion]"
func printVersionUpdate(ctx context.Context, oldVersion semver.Version) error {
	io := iostreams.FromContext(ctx)

	currentVer, err := getNewVersion(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "failed to parse version") {
			// This is probably fine, likely a change between the two versions makes
			// flyctl <-> flyctl communication incompatible
			return nil
		} else {
			return err
		}
	}

	if currentVer.EQ(oldVersion) {
		fmt.Fprintf(io.ErrOut, "Flyctl was updated, but the flyctl pointed to by '%s' is still version %s.\n", os.Args[0], currentVer.String())
		fmt.Fprintf(io.ErrOut, "Please ensure that your PATH is set correctly!")
		return nil
	}

	fmt.Fprintf(io.Out, "Updated flyctl v%s -> v%s\n", oldVersion.String(), currentVer.String())
	return nil
}

func getNewVersionHomebrew(ctx context.Context) (semver.Version, error) {

	var ver semver.Version

	newVersionJson, err := exec.CommandContext(ctx, "brew", "info", "flyctl", "--json").CombinedOutput()
	if err != nil {
		return ver, fmt.Errorf("failed to query version information from homebrew: %w", err)
	}

	var parsed []map[string]any
	if err = json.Unmarshal(newVersionJson, &parsed); err != nil {
		return ver, fmt.Errorf("failed to parse version output from brew: %w", err)
	}

	versions := lo.Map(parsed, func(def map[string]any, _ int) []*semver.Version {
		if def["name"] != "flyctl" {
			return nil
		}
		installed, ok := def["installed"].([]any)
		if !ok {
			return nil
		}
		return lo.FilterMap(installed, func(defAny any, _ int) (*semver.Version, bool) {
			version, ok := defAny.(map[string]any)["version"].(string)
			if !ok {
				return nil, false
			}
			parsed, err := semver.ParseTolerant(version)
			if err != nil {
				return nil, false
			}
			return &parsed, true
		})
	})
	versionsFlat := lo.Map(lo.Flatten(versions), func(v *semver.Version, _ int) semver.Version { return *v })
	semver.Sort(versionsFlat)

	if len(versionsFlat) == 0 {
		return ver, errors.New("brew reports no installed flyctl version")
	}
	return versionsFlat[len(versionsFlat)-1], nil
}

// getNewVersion executes [os.Args[0], "version", "--json"] and parses the output into a semver.Version
func getNewVersion(ctx context.Context) (semver.Version, error) {

	if update.IsUnderHomebrew() {
		return getNewVersionHomebrew(ctx)
	}

	var ver semver.Version

	newVersionJson, err := exec.CommandContext(ctx, os.Args[0], "version", "--json").CombinedOutput()
	if err != nil {
		return ver, fmt.Errorf("failed to execute new flyctl binary: %w", err)
	}
	// Parsing into a map instead of the struct directly so that
	// small changes in the version struct don't break this.
	parsed := map[string]string{}
	if err = json.Unmarshal(newVersionJson, &parsed); err != nil {
		return ver, fmt.Errorf("failed to parse version of new flyctl binary: %w", err)
	}
	semverStr, ok := parsed["Version"]
	if !ok {
		return ver, errors.New("failed to parse version of new flyctl binary: field 'Version' not in output of 'fly version --json'")
	}
	ver, err = semver.ParseTolerant(semverStr)
	if err != nil {
		return ver, fmt.Errorf("failed to parse version of new flyctl binary: %w", err)
	}
	return ver, nil
}
