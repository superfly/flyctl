package flyctl

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func CheckForUpdate() {
	if skipUpdateCheck() {
		return
	}

	if updateAvailable() {
		latestVersion := viper.GetString(ConfigUpdateCheckLatestVersion)
		fmt.Println(aurora.Yellow(fmt.Sprintf("Update available %s -> %s", Version, latestVersion)))
	}

	lastCheck := viper.GetTime(ConfigUpdateCheckTimestamp)
	if lastCheck.Add(1 * time.Hour).Before(time.Now()) {
		go checkForRelease()
	}
}

func checkForRelease() {
	BackgroundTaskWG.Add(1)
	defer BackgroundTaskWG.Done()

	if version, err := refreshGithubVersion(); err == nil {
		viper.Set(ConfigUpdateCheckLatestVersion, version)
		viper.Set(ConfigUpdateCheckTimestamp, time.Now())
		SaveConfig()
	}
}

type githubReleaseResponse struct {
	Name string
}

func refreshGithubVersion() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/superfly/flyctl/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data := githubReleaseResponse{}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	return strings.TrimPrefix(data.Name, "v"), nil
}
