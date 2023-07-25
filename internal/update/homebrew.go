package update

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cli/safeexec"
	"github.com/superfly/flyctl/terminal"
)

var errBrewNotFound = errors.New("command 'brew' not found")

// Use brewBinDir()
var _brewBinDir memoize[string]

func brewBinDir() (string, error) {

	return _brewBinDir.Get(func() (string, error) {

		brewExe, err := safeexec.LookPath("brew")
		if err != nil {
			return "", errBrewNotFound
		}

		brewPrefixBytes, err := exec.Command(brewExe, "--prefix").Output()
		if err != nil {
			return "", err
		}

		brewBinPrefix := filepath.Join(strings.TrimSpace(string(brewPrefixBytes)), "bin") + string(filepath.Separator)

		return brewBinPrefix, nil
	})

}

// Use IsUnderHomebrew()
var _isUnderHomebrew memoize[bool]

// IsUnderHomebrew reports whether the fly binary was found under the Homebrew
// prefix.
func IsUnderHomebrew() bool {

	if runtime.GOOS == "windows" {
		return false
	}

	val, err := _isUnderHomebrew.Get(func() (bool, error) {
		flyBinary, err := os.Executable()
		if err != nil {
			return false, err
		}

		brewBinPrefix, err := brewBinDir()
		if err != nil {
			return false, err
		}

		return strings.HasPrefix(flyBinary, brewBinPrefix), nil
	})
	if err != nil {
		return false
	}
	return val
}

var _latestHomebrewRelease memoize[*Release]

func latestHomebrewRelease(ctx context.Context) (*Release, error) {

	return _latestHomebrewRelease.Get(func() (*Release, error) {

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
	})
}

func updateHomebrewCache(ctx context.Context, remoteRelease *Release) error {

	brewExe, err := safeexec.LookPath("brew")
	if err != nil {
		return errBrewNotFound
	}

	infoJsonBytes, err := exec.Command(brewExe, "info", "flyctl", "--json").Output()
	if err != nil {
		return err
	}

	var infoJson []struct {
		Versions struct {
			Stable string `json:"stable"`
		} `json:"versions"`
	}
	err = json.Unmarshal(infoJsonBytes, &infoJson)
	if err != nil {
		return err
	}
	if len(infoJson) != 1 {
		return errors.New("unexpected output length from 'brew info flyctl --json'")
	}
	localStable := infoJson[0].Versions.Stable
	if localStable == remoteRelease.Version {
		return nil
	}

	terminal.Debugf("updating homebrew cache for flyctl %s\n", remoteRelease.Version)
	return exec.Command(brewExe, "fetch", "--force", "flyctl").Run()
}
