package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cli/safeexec"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/pkg/iostreams"
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
	updateUrl := fmt.Sprintf("https://api.fly.io/app/flyctl_releases/%s/%s/%s", runtime.GOOS, runtime.GOARCH, channel)
	req, err := http.NewRequestWithContext(ctx, "GET", updateUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return &release, err
	}

	return &release, nil
}

// isUnderHomebrew reports whether the fly binary was found under the Homebrew
// prefix.
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

func UpgradeInPlace(ctx context.Context, io *iostreams.IOStreams, prelease bool) error {
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

	command := updateCommand(prelease)

	fmt.Fprintf(io.ErrOut, "Running automatic update [%s]\n", command)

	cmd := exec.Command(shellToUse, switchToUse, command)
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

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
