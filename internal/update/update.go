package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/cli/safeexec"
	"github.com/morikuni/aec"
	"github.com/superfly/flyctl/terminal"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/iostreams"
)

type Release struct {
	Version     string    `yaml:"version"`
	Prerelease  bool      `yaml:"prerelease"`
	DownloadURL string    `yaml:"download_url" json:"download_url"`
	Timestamp   time.Time `yaml:"timestamp"`
}

// Check reports whether update checks should take place.
func Check() bool {
	switch {
	case env.IsTruthy("FLY_UPDATE_CHECK"):
		return true
	case env.IsTruthy("FLY_NO_UPDATE_CHECK"):
		return false
	case env.IsSet("CODESPACES"):
		return false
	case !buildinfo.IsRelease(), env.IsCI():
		return false
	case !cmdutil.IsTerminal(os.Stdout), !cmdutil.IsTerminal(os.Stderr):
		return false
	default:
		return true
	}
}

// LatestRelease reports the latest release for the given channel.
func LatestRelease(ctx context.Context, channel string) (*Release, error) {

	// If running under homebrew, use the homebrew API to get the latest release
	if IsUnderHomebrew() {
		return latestHomebrewRelease(ctx, channel)
	}
	return latestApiRelease(ctx, channel)
}

func upgradeCommand(prerelease bool) string {
	if IsUnderHomebrew() {
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

func UpgradeInPlace(ctx context.Context, io *iostreams.IOStreams, prelease, silent bool) error {
	if runtime.GOOS == "windows" {
		if err := renameCurrentBinaries(); err != nil {
			return err
		}
	}

	if IsUnderHomebrew() {

		brewExe, err := safeexec.LookPath("brew")
		if err == nil {
			err = exec.Command(brewExe, "update").Run()
		}
		if err != nil {
			terminal.Debugf("error updating homebrew cache: %s", err)
		}
	}

	var shellToUse string
	switchToUse := "-c"
	ok := false

	if runtime.GOOS != "windows" {
		shellToUse, ok = os.LookupEnv("SHELL")
	}

	if !ok {
		if runtime.GOOS == "windows" {
			// pwsh.exe is the name of the PowerShell executable from 6.0+
			// powershell.exe is locked to 5.1 forever
			if commandInPath("pwsh.exe") {
				shellToUse = "pwsh.exe"
				switchToUse = "-Command"
			} else {
				shellToUse = "powershell.exe"
				switchToUse = "-Command"
			}
		} else {
			shellToUse = "/bin/bash"
		}
	}

	command := upgradeCommand(prelease)
	cmd := exec.Command(shellToUse, switchToUse, command)

	if !silent {
		fmt.Fprintf(io.ErrOut, "Running automatic upgrade [%s]\n", command)

		cmd.Stdout = io.Out
		cmd.Stderr = io.ErrOut
		cmd.Stdin = io.In
	}

	err := cmd.Run()
	if err != nil {
		return err
	}

	// Remove the line that says `Run 'flyctl --help' to get started`
	if !IsUnderHomebrew() && io.IsInteractive() && !silent {
		builder := aec.EmptyBuilder
		str := builder.Up(1).EraseLine(aec.EraseModes.All).ANSI
		fmt.Fprint(io.ErrOut, str.String())
	}
	return nil
}

func GetCurrentBinaryPath() (string, error) {

	if IsUnderHomebrew() {
		brewBinPrefix, err := brewBinDir()
		if err != nil {
			return "", err
		}

		homebrewFlyctl := filepath.Join(brewBinPrefix, "flyctl")
		if _, err := os.Stat(homebrewFlyctl); err == nil {
			return homebrewFlyctl, nil
		}
		// Not linked (?), use fallback method
	}

	binPath, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}

	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return "", err
	}
	return binPath, nil
}

// Relaunch only returns on error
func Relaunch(ctx context.Context, silent bool) error {

	io := iostreams.FromContext(ctx)

	if !silent {
		// Divide between update logging and the output from the actual command the user is running
		fmt.Fprint(io.Out, "\n----\n\n")
	}

	// Wait a bit for the update to take effect.
	// Windows seemed to need this for whatever reason.
	time.Sleep(400 * time.Millisecond)

	binPath, err := GetCurrentBinaryPath()
	if err != nil {
		return err
	}

	terminal.Debugf("relaunching %s, found at %s\n", os.Args[0], binPath)

	cmd := exec.Command(binPath, os.Args[1:]...)
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In
	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, "FLY_NO_UPDATE_CHECK=1")

	if err := cmd.Start(); err != nil {
		return err
	}

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}

	os.Exit(0)
	return nil
}

func commandInPath(command string) bool {
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		path := filepath.Join(dir, command)
		if _, err := os.Stat(path); err == nil {
			return true
		}
	}

	return false
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

func currentWindowsBinaries() ([]string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	canonicalPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return nil, err
	}

	return []string{
		canonicalPath,
		filepath.Join(filepath.Dir(canonicalPath), "wintun.dll"),
	}, nil
}

// BackgroundUpdate begins an update in the background.
func BackgroundUpdate() error {

	binPath, err := exec.LookPath(os.Args[0])
	if err != nil {
		return err
	}
	terminal.Debugf("launching `%s version update` with binary %s\n", os.Args[0], binPath)

	cmd := exec.Command(binPath, "version", "upgrade")
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}
