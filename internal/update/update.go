package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/blang/semver"
	"github.com/cli/safeexec"
	"github.com/superfly/flyctl/terminal"
)

// Check whether the fly binary was found under the Homebrew prefix
func isUnderHomebrew() bool {
	flyBinary, err := os.Executable()
	if err != nil {
		return false
	}

	brewExe, err := safeexec.LookPath("brew")
	if err != nil {
		return false
	}

	brewPrefixBytes, err := exec.Command(brewExe, "--prefix").Output()
	if err != nil {
		return false
	}

	brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)
	return strings.HasPrefix(flyBinary, brewBinPrefix)
}

func updateCommand(prerelease bool) string {
	if isUnderHomebrew() {
		return "brew upgrade flyctl"
	}

	if runtime.GOOS == "windows" {
		cmd := "iwr https://fly.io/install.ps1 -useb | iex"
		if prerelease {
			cmd = "$v=\"pre\"; " + cmd
		}
		return cmd
	} else {
		cmd := "curl -L \"https://fly.io/install.sh\" | sh"
		if prerelease {
			cmd = cmd + " -s pre"
		}
		return cmd
	}
}

func PerformInPlaceUpgrade(ctx context.Context, configPath string, currentVersion semver.Version) error {
	state, _ := loadState(configPath)
	if state.Channel == "" {
		state.Channel = "latest"
	}

	release, err := fetchLatestVersion(ctx, state.Channel)
	if err != nil {
		return err
	}

	var latestVersion semver.Version
	if release != nil {
		latestVersion, err = semver.ParseTolerant(release.Version)
		if err != nil {
			terminal.Warnf("error parsing version number '%s': %s\n", state.LatestRelease.Version, err)
		}
		// if latestVersion.GT(currentVersion) {
		// 	return state.LatestRelease, nil
		// }
	}

	if release == nil || !latestVersion.GT(currentVersion) {
		fmt.Println("No update available")
		return nil
	}

	if runtime.GOOS == "windows" {
		if err := renameCurrentBinaries(); err != nil {
			return err
		}
	}

	shellToUse, ok := os.LookupEnv("SHELL")
	switchToUse := "-c"

	if !ok {
		if runtime.GOOS == "windows" {
			shellToUse = "powershell.exe"
			switchToUse = "-Command"
		} else {
			shellToUse = "/bin/bash"
		}
	}
	fmt.Println(shellToUse, switchToUse)

	command := updateCommand(release.Prerelease)

	fmt.Println("Running automatic update [" + command + "]")
	cmd := exec.Command(shellToUse, switchToUse, command)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	return cmd.Run()
}

// can't replace binary on windows, need to move
func renameCurrentBinaries() error {
	binaries, err := currentWindowsBinaries()
	if err != nil {
		return err
	}

	for _, p := range binaries {
		if err := os.Rename(p, p+".old"); err != nil {
			return err
		}
	}

	return nil
}

func PostUpgradeCleanup() error {
	if runtime.GOOS != "windows" {
		return nil
	}

	binaries, err := currentWindowsBinaries()
	if err != nil {
		return err
	}

	for _, p := range binaries {
		os.Remove(p + ".old")
	}

	return nil
}

func currentWindowsBinaries() ([]string, error) {
	binaryPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	return []string{
		binaryPath,
		filepath.Join(filepath.Dir(binaryPath), "wintun.dll"),
	}, nil
}
