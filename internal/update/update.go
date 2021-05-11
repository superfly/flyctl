package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cli/safeexec"
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

func PerformInPlaceUpgrade(ctx context.Context, configPath string, currentVersion string) error {
	release, err := CheckForUpdate(ctx, configPath, currentVersion)
	if err != nil {
		return err
	}

	if release == nil {
		fmt.Println("No update available")
		return nil
	}

	if runtime.GOOS == "windows" {
		// can't replace binary on windows, need to move
		binaryPath, err := os.Executable()
		if err != nil {
			return err
		}

		toMove := []string{
			binaryPath,
			filepath.Join(filepath.Dir(binaryPath), "wintun.dll"),
		}

		for _, p := range toMove {
			if err := os.Rename(p, p+".old"); err != nil {
				return err
			}
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
