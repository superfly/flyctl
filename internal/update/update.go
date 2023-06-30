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
	updateUrl := fmt.Sprintf("https://api.fly.io/app/flyctl_releases/%s/%s/%s", runtime.GOOS, runtime.GOARCH, channel)

	// If running under homebrew, use the homebrew API to get the latest release
	if IsUnderHomebrew() {
		return latestHomebrewRelease(ctx, channel)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", updateUrl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			terminal.Debugf("error closing response body: %s", err)
		}
	}()

	var release Release
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return &release, err
	}

	return &release, nil
}

func latestHomebrewRelease(ctx context.Context, channel string) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://formulae.brew.sh/api/formula/flyctl.json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			terminal.Debugf("error closing response body: %s", err)
		}
	}()

	var brewResp struct {
		Versions struct {
			Stable string `json:"stable"`
		} `json:"versions"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&brewResp); err != nil {
		return nil, err
	}

	return &Release{
		Version: brewResp.Versions.Stable,
	}, nil
}

// IsUnderHomebrew reports whether the fly binary was found under the Homebrew
// prefix.
func IsUnderHomebrew() bool {
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
	fmt.Println(shellToUse, switchToUse)

	command := upgradeCommand(prelease)
	cmd := exec.Command(shellToUse, switchToUse, command)

	if !silent {
		fmt.Fprintf(io.ErrOut, "Running automatic upgrade [%s]\n", command)

		cmd.Stdout = io.Out
		cmd.Stderr = io.ErrOut
		cmd.Stdin = io.In
	}

	return cmd.Run()
}

// Relaunch only returns on error
func Relaunch(ctx context.Context, silent bool) error {

	io := iostreams.FromContext(ctx)

	if !silent {
		// Divide between update logging and the output from the actual command the user is running
		fmt.Fprint(io.Out, "\n----\n\n")
	}

	// Wait a bit for the update to take effect.
	// Windows seemed to need this.
	time.Sleep(500 * time.Millisecond)

	binPath, err := exec.LookPath(os.Args[0])
	if err != nil {
		return err
	}

	binPath, err = filepath.EvalSymlinks(binPath)
	if err != nil {
		return err
	}
	terminal.Debugf("relaunching %s, found at %s\n", os.Args[0], binPath)

	cmd := exec.Command(binPath, os.Args[1:]...)
	cmd.Stdout = io.Out
	cmd.Stderr = io.ErrOut
	cmd.Stdin = io.In

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

type deferredUpdateDef struct {
	prerel    bool
	silent    bool
	iostreams *iostreams.IOStreams
}

var deferredUpdate *deferredUpdateDef

// BackgroundUpdate begins an update in the background.
func BackgroundUpdate(ctx context.Context, prerel, silent bool) error {

	if runtime.GOOS == "windows" {
		// On Windows, updates are slow and display a UAC prompt.
		// This is pretty awful UX, so we defer the update until execution is over.
		deferredUpdate = &deferredUpdateDef{
			prerel,
			silent,
			iostreams.FromContext(ctx),
		}
		return nil
	}

	binPath, err := exec.LookPath(os.Args[0])
	if err != nil {
		return err
	}
	terminal.Debugf("launching `%s version update` with binary %s\n", os.Args[0], binPath)

	cmd := exec.Command(binPath, "version", "upgrade")

	if err := cmd.Start(); err != nil {
		return err
	}
	return nil
}

func DoDeferredUpdate() error {
	if deferredUpdate == nil {
		return nil
	}
	return UpgradeInPlace(context.Background(), deferredUpdate.iostreams, deferredUpdate.prerel, deferredUpdate.silent)
}
