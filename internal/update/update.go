package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cli/safeexec"
	"github.com/morikuni/aec"
	"github.com/superfly/flyctl/api"
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

type InvalidReleaseError struct {
	status int
	msg    string
}

func (i InvalidReleaseError) Error() string {
	return i.msg
}
func (i InvalidReleaseError) StatusCode() int {
	return i.status
}

// memoized values for ValidateRelease
var validatedReleases = map[string]error{}

// ValidateRelease reports whether the given release is valid via an API call.
// If the version is invalid, the error will be an InvalidReleaseError.
// Note that other errors may be returned if the API call fails.
func ValidateRelease(ctx context.Context, version string) (err error) {

	if version[0] == 'v' {
		version = version[1:]
	}

	if err, ok := validatedReleases[version]; ok {
		return err
	}

	defer func() {
		validatedReleases[version] = err
	}()

	updateUrl := fmt.Sprintf("https://api.fly.io/app/flyctl_validate/v%s", version)

	req, err := http.NewRequestWithContext(ctx, "GET", updateUrl, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Accept", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			terminal.Debugf("error closing response body: %s", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		return &InvalidReleaseError{
			status: resp.StatusCode,
			msg:    string(body),
		}
	}

	return nil
}

// LatestRelease reports the latest release for the given channel.
func LatestRelease(ctx context.Context, channel string) (*Release, error) {

	// If running under homebrew, use the homebrew API to get the latest release
	if IsUnderHomebrew() {
		return latestHomebrewRelease(ctx, channel)
	}

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

// memoized value so we're not executing homebrew commands on every call
// Use IsUnderHomebrew()
var isUnderHomebrew *bool

// IsUnderHomebrew reports whether the fly binary was found under the Homebrew
// prefix.
func IsUnderHomebrew() bool {

	if isUnderHomebrew != nil {
		return *isUnderHomebrew
	}

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

	isUnderHomebrew = api.BoolPointer(strings.HasPrefix(flyBinary, brewBinPrefix))
	return *isUnderHomebrew
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
	// Windows seemed to need this for whatever reason.
	time.Sleep(250 * time.Millisecond)

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

	// Remove the line that says `Run 'flyctl --help' to get started`
	if io.IsInteractive() {
		builder := aec.EmptyBuilder
		str := builder.Up(1).EraseLine(aec.EraseModes.All).ANSI
		fmt.Fprint(io.ErrOut, str.String())
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
