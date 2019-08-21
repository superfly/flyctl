package flyctl

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/blang/semver"
	"github.com/logrusorgru/aurora"
)

var BackgroundTaskWG = &sync.WaitGroup{}

type UpdateStatus struct {
	LastUpdateCheck time.Time
	LatestVersion   string
	OptOut          bool
}

func (status UpdateStatus) updateAvailable() bool {
	if status.LatestVersion == "" || Version == "" {
		return false
	}

	latestVersion, err := semver.Parse(status.LatestVersion)
	if err != nil {
		return false
	}
	currentVersion, err := semver.Parse(Version)
	if err != nil {
		return false
	}

	return latestVersion.GT(currentVersion)
}

func CheckForUpdate() {
	us := readCachedUpdateStatus()

	if us.OptOut || Version == "" {
		return
	}

	go refreshUpdateStatus(us)

	if us.updateAvailable() {
		fmt.Println(aurora.Yellow(fmt.Sprintf("Update available %s -> %s", Version, us.LatestVersion)))
	}

	if us.LatestVersion == "" || Version == "" {
		return
	}

}

func refreshUpdateStatus(us UpdateStatus) {
	if us.LastUpdateCheck.Add(1 * time.Hour).After(time.Now()) {
		return
	}

	BackgroundTaskWG.Add(1)
	defer BackgroundTaskWG.Done()

	if version, err := refreshGithubVersion(); err == nil {
		us.LatestVersion = version
		us.LastUpdateCheck = time.Now()
		writeCachedUpdateStatus(us)
	}
}

func readCachedUpdateStatus() UpdateStatus {
	updateStatus := UpdateStatus{}

	configDir, err := ConfigDir()
	if err != nil {
		return UpdateStatus{}
	}

	configFile := path.Join(configDir, "update.json")

	bytes, err := ioutil.ReadFile(configFile)
	if err != nil {
		return updateStatus
	}

	json.Unmarshal(bytes, &updateStatus)

	return updateStatus
}

func writeCachedUpdateStatus(status UpdateStatus) error {
	configDir, err := ConfigDir()
	if err != nil {
		return nil
	}

	configFile := path.Join(configDir, "update.json")

	bytes, err := json.Marshal(status)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(configFile, bytes, 0644)
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
