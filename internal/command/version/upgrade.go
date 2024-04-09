package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/exp/slices"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cache"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/update"
	"github.com/superfly/flyctl/internal/version"
	"github.com/superfly/flyctl/iostreams"
)

func newUpgrade() *cobra.Command {
	const (
		short = "Checks for available updates and automatically upgrades"

		long = `Checks for an update and if one is available, runs the appropriate
command to upgrade the application.`
	)

	cmd := command.New("upgrade", short, long, runUpgrade)

	cmd.Aliases = []string{"update"}

	return cmd
}

func runUpgrade(ctx context.Context) error {
	release, err := update.LatestRelease(ctx, cache.FromContext(ctx).Channel())
	switch {
	case err != nil:
		return fmt.Errorf("failed determining latest release: %w", err)
	case release == nil:
		return fmt.Errorf("failed querying latest release information: %w", err)
	}

	// The API won't return yanked versions, but we don't have a good way
	// to yank homebrew releases. If we're under homebrew, we'll validate through the API
	if update.IsUnderHomebrew() {
		if relErr := update.ValidateRelease(ctx, release.Version); relErr != nil {
			return fmt.Errorf("latest version on homebrew is invalid: %s\nplease try again later", relErr)
		}
	}

	latest, err := version.Parse(release.Version)
	if err != nil {
		return fmt.Errorf("error parsing version: %q, %w", release.Version, err)
	}

	io := iostreams.FromContext(ctx)

	if !latest.Newer(buildinfo.Version()) {
		fmt.Fprintf(io.Out, "Already running latest flyctl v%s\n", buildinfo.Version().String())
		return nil
	}

	if !update.CanUpdateThisInstallation() {
		return errors.New("cannot update this installation.\nthe environment variable FLYCTL_INSTALL must be set to the installation directory")
	}

	homebrew := update.IsUnderHomebrew()

	if err = update.UpgradeInPlace(ctx, io, release.Prerelease, false); err != nil {
		return err
	}

	err = printVersionUpgrade(ctx, buildinfo.Version(), homebrew)
	if err != nil {
		terminal.Debugf("Error printing version upgrade: %v", err)
	}
	return nil
}

// printVersionUpgrade prints "Upgraded flyctl [oldVersion] -> [newVersion]"
func printVersionUpgrade(ctx context.Context, oldVersion version.Version, homebrew bool) error {

	var (
		io         = iostreams.FromContext(ctx)
		currentVer version.Version
		err        error
	)

	if homebrew {
		currentVer, err = getNewVersionHomebrew(ctx)
	} else {
		currentVer, err = getNewVersionFlyInstaller(ctx)
	}
	if err != nil {
		if strings.Contains(err.Error(), "failed to parse version") {
			// This is probably fine, likely a change between the two versions makes
			// flyctl <-> flyctl communication incompatible
			return nil
		} else {
			return err
		}
	}

	if currentVer.Equal(oldVersion) {
		var source string
		if homebrew {
			source = "homebrew"
		} else {
			source = fmt.Sprintf("'%s'", os.Args[0])
		}
		fmt.Fprintf(io.ErrOut, "Flyctl was upgraded, but the flyctl pointed to by %s is still version %s.\n", source, currentVer.String())
		fmt.Fprintf(io.ErrOut, "Please ensure that your PATH is set correctly!")
		return nil
	}

	fmt.Fprintf(io.Out, "Upgraded flyctl v%s -> v%s\n", oldVersion.String(), currentVer.String())
	return nil
}

// getNewVersionFlyInstaller queries homebrew for the latest currently installed version of flyctl
// It parses the output of `brew info flyctl --json`
func getNewVersionHomebrew(ctx context.Context) (version.Version, error) {
	var ver version.Version

	newVersionJson, err := exec.CommandContext(ctx, "brew", "info", "flyctl", "--json").CombinedOutput()
	if err != nil {
		return ver, fmt.Errorf("failed to query version information from homebrew: %w", err)
	}

	var parsed []map[string]any
	if err = json.Unmarshal(newVersionJson, &parsed); err != nil {
		return ver, fmt.Errorf("failed to parse version output from brew: %w", err)
	}

	versions := lo.Map(parsed, func(def map[string]any, _ int) []*version.Version {
		if def["name"] != "flyctl" {
			return nil
		}
		installed, ok := def["installed"].([]any)
		if !ok {
			return nil
		}
		return lo.FilterMap(installed, func(defAny any, _ int) (*version.Version, bool) {
			v, ok := defAny.(map[string]any)["version"].(string)
			if !ok {
				return nil, false
			}
			parsed, err := version.Parse(v)
			if err != nil {
				return nil, false
			}
			return &parsed, true
		})
	})
	versionsFlat := lo.Map(lo.Flatten(versions), func(v *version.Version, _ int) version.Version { return *v })
	slices.SortFunc(versionsFlat, version.Compare)

	if len(versionsFlat) == 0 {
		return ver, errors.New("brew reports no installed flyctl version")
	}
	return versionsFlat[len(versionsFlat)-1], nil
}

// getNewVersionFlyInstaller executes [os.Args[0], "version", "--json"] and parses the output into a semver.Version
func getNewVersionFlyInstaller(ctx context.Context) (version.Version, error) {
	var ver version.Version

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
	verStr, ok := parsed["Version"]
	if !ok {
		return ver, errors.New("failed to parse version of new flyctl binary: field 'Version' not in output of 'fly version --json'")
	}
	ver, err = version.Parse(verStr)
	if err != nil {
		return ver, fmt.Errorf("failed to parse version of new flyctl binary: %w", err)
	}
	return ver, nil
}
