package flyctl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/viper"
)

var BackgroundTaskWG = &sync.WaitGroup{}

func skipUpdateCheck() bool {
	return Version == "" || viper.GetBool(ConfigUpdateCheckOptOut)
}

func updateAvailable() bool {
	if !viper.IsSet(ConfigUpdateCheckLatestVersion) {
		return false
	}

	lv, err := semver.Parse(viper.GetString(ConfigUpdateCheckLatestVersion))
	if err != nil {
		return false
	}
	cv, err := semver.Parse(Version)
	if err != nil {
		return false
	}

	return lv.GT(cv)
}

// CheckForUpdate - Test for available updates and emit a message if one is available
func CheckForUpdate() {
	if skipUpdateCheck() {
		return
	}

	if updateAvailable() {
		latestVersion := viper.GetString(ConfigUpdateCheckLatestVersion)
		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Update available %s -> %s", Version, latestVersion)))
	}

	lastCheck := viper.GetTime(ConfigUpdateCheckTimestamp)
	if lastCheck.Add(1 * time.Hour).Before(time.Now()) {
		BackgroundTaskWG.Add(1)
		go checkForRelease()
	}
}

func checkForRelease() {
	defer BackgroundTaskWG.Done()

	if version, err := refreshGithubVersion(); err == nil {
		viper.Set(ConfigUpdateCheckLatestVersion, version)
		viper.Set(ConfigUpdateCheckTimestamp, time.Now())
		SaveConfig()
	}
}

type githubReleaseLatestResponse struct {
	Name string
}

type githubReleaseResponse []githubReleaseLatestResponse

func refreshGithubVersion() (string, error) {
	cv, err := semver.Parse(Version)
	if err != nil {
		return "", err
	}
	var resp *http.Response

	if len(cv.Pre) == 0 {
		resp, err = http.Get("https://api.github.com/repos/superfly/flyctl/releases/latest")
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		data := githubReleaseLatestResponse{}

		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			fmt.Println(err)
			return "", err
		}
		return strings.TrimPrefix(data.Name, "v"), nil
	}

	resp, err = http.Get("https://api.github.com/repos/superfly/flyctl/releases")
	if err != nil {
		return "", err
	}

	data := githubReleaseResponse{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		fmt.Println(err)
		return "", err
	}

	return strings.TrimPrefix(data[0].Name, "v"), nil

}
