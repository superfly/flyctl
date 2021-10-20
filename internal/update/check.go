package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/logrusorgru/aurora"
	"gopkg.in/yaml.v2"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/terminal"
)

type Release struct {
	Version     string    `yaml:"version"`
	Prerelease  bool      `yaml:"prerelease"`
	DownloadURL string    `yaml:"download_url" json:"download_url"`
	Timestamp   time.Time `yaml:"timestamp"`
}

type state struct {
	Channel       string    `yaml:"channel,omitempty"`
	LastCheckedAt time.Time `yaml:"last_checked_at,omitempty"`
	LatestRelease *Release  `yaml:"latest_release,omitempty"`
}

func InitState(configPath string, channel string) error {
	state, _ := loadState(configPath)

	if strings.Contains(channel, "pre") {
		channel = "pre"
	} else {
		channel = "latest"
	}

	state.Channel = channel

	return saveState(configPath, state)
}

// PromptFor prompts the user to update flyctl should a newer version be available.
func PromptFor(ctx context.Context) {
	if !shouldCheckForUpdate() {
		return
	}

	terminal.Debug("Checking for updates...")

	currentVersion := buildinfo.Version()
	stateFilePath := filepath.Join(flyctl.ConfigDir(), "state.yml")

	newVersion, err := checkForUpdate(ctx, stateFilePath, currentVersion)
	switch {
	case err == nil:
		// newer version detected
	case flyerr.IsCancelledError(err):
		return
	default:
		terminal.Errorf("error checking for update: %v", err)

		return
	}

	msg := fmt.Sprintf("Update available %s -> %s.\nRun \"%s\" to upgrade.",
		currentVersion,
		newVersion.Version,
		aurora.Bold(buildinfo.Name()+" version update"),
	)
	fmt.Fprintln(os.Stderr, aurora.Yellow(msg))
}

func shouldCheckForUpdate() bool {
	// for testing
	if os.Getenv("FLY_UPDATE_CHECK") == "1" {
		return true
	}

	if os.Getenv("FLY_NO_UPDATE_CHECK") != "" {
		return false
	}
	if os.Getenv("CODESPACES") != "" {
		return false
	}

	if !buildinfo.IsRelease() || env.IsCI() || !cmdutil.IsTerminal(os.Stdout) || !cmdutil.IsTerminal(os.Stderr) {
		return false
	}

	return true
}

func checkForUpdate(ctx context.Context, configPath string, currentVersion semver.Version) (*Release, error) {
	state, _ := loadState(configPath)
	if state.Channel == "" {
		state.Channel = "latest"
	}

	if state.LatestRelease == nil || time.Since(state.LastCheckedAt).Hours() > 1 {
		release, err := fetchLatestVersion(ctx, state.Channel)
		if err != nil {
			return nil, err
		}
		state.LatestRelease = release
		state.LastCheckedAt = time.Now()
		if err := saveState(configPath, state); err != nil {
			return nil, err
		}
	}

	if state.LatestRelease != nil {
		latestVersion, err := semver.ParseTolerant(state.LatestRelease.Version)
		if err != nil {
			terminal.Warnf("error parsing version number '%s': %s\n", state.LatestRelease.Version, err)
			return nil, nil
		}
		if latestVersion.GT(currentVersion) {
			return state.LatestRelease, nil
		}
	}

	return nil, nil
}

func fetchLatestVersion(ctx context.Context, channel string) (*Release, error) {
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

func loadState(filename string) (state, error) {
	var s state
	data, err := os.ReadFile(filename)
	if err != nil {
		return s, err
	}

	if err := yaml.Unmarshal(data, &s); err != nil {
		return s, err
	}

	return s, nil
}

func saveState(filename string, state state) error {
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0600)
}
