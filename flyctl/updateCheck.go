package flyctl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/superfly/flyctl/flyname"

	"github.com/blang/semver"
	"github.com/logrusorgru/aurora"
	"github.com/spf13/viper"
)

var BackgroundTaskWG = &sync.WaitGroup{}

func skipUpdateCheck() bool {
	return Version == "" || viper.GetBool(ConfigUpdateCheckOptOut)
}

func updateAvailable() bool {
	fmt.Println("In Update Available")
	if !viper.IsSet(ConfigUpdateCheckLatestVersion) {
		return false
	}

	lv, err := semver.ParseTolerant(viper.GetString(ConfigUpdateCheckLatestVersion))
	if err != nil {
		return false
	}

	TestVersion := Version

	if TestVersion == "<version>" {
		TestVersion = "0.0.0"
	}
	cv, err := semver.ParseTolerant(TestVersion)

	if err != nil {
		fmt.Println(err)
		return false
	}

	return lv.GT(cv)
}

// CheckForUpdate - Test for available updates and emit a message if one is available
func CheckForUpdate(noskip bool, silent bool) string {
	name, _ := os.Executable()
	if (!noskip && skipUpdateCheck()) || filepath.Base(name) == "main" {
		return ""
	}

	checkForReleaseBlocking()

	if updateAvailable() {
		latestVersion := viper.GetString(ConfigUpdateCheckLatestVersion)
		installer := viper.GetString(ConfigInstaller)

		fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Update available %s -> %s", Version, latestVersion)))

		var installerstring string
		if installer != "" {
			if runtime.GOOS == "windows" {
				switch installer {

				case "shell":
					installerstring = "iwr https://fly.io/install.ps1 | iex"
				case "shell-prerel":
					installerstring = "iwr https://fly.io/installprerel.ps1 | iex"
				}
			} else {
				switch installer {

				case "shell":
					installerstring = "curl -L \"https://fly.io/install.sh\" | sh"
				case "shell-prerel":
					installerstring = "curl -L \"https://fly.io/install.sh\" | sh -s prerel"
				}
			}
		} else {
			if runtime.GOOS == "darwin" {
				installerstring = "brew upgrade flyctl"
			} else if runtime.GOOS == "windows" {
				installerstring = "iwr https://fly.io/install.ps1 | iex"
			} else {
				installerstring = "curl -L \"https://fly.io/install.sh\" | sh"
			}
		}
		if !silent {
			fmt.Fprintln(os.Stderr, aurora.Yellow(fmt.Sprintf("Update with %s version update\n", flyname.Name())))
		}
		return installerstring
	}

	lastCheck := viper.GetTime(ConfigUpdateCheckTimestamp)
	if lastCheck.Add(1 * time.Hour).Before(time.Now()) {
		BackgroundTaskWG.Add(1)
		go checkForRelease()
	}

	return ""
}

func checkForRelease() {
	defer BackgroundTaskWG.Done()

	if version, err := refreshGithubVersion(); err == nil {
		viper.Set(ConfigUpdateCheckLatestVersion, version)
		viper.Set(ConfigUpdateCheckTimestamp, time.Now())
		err := SaveConfig()
		if err != nil {
			fmt.Printf("Error saving config while checking release:%s", err)
			return
		}
	}
}

func checkForReleaseBlocking() {
	if version, err := refreshGithubVersion(); err == nil {
		viper.Set(ConfigUpdateCheckLatestVersion, version)
		viper.Set(ConfigUpdateCheckTimestamp, time.Now())
		err := SaveConfig()
		if err != nil {
			fmt.Printf("Error saving config while checking release:%s", err)
			return
		}
	}
}

type githubReleaseLatestResponse struct {
	Name string
}

type githubReleaseResponse []githubReleaseLatestResponse

func refreshGithubVersion() (string, error) {
	TestVersion := Version

	if TestVersion == "<version>" {
		TestVersion = "0.0.0"
	}

	cv, err := semver.ParseTolerant(TestVersion)
	if err != nil {
		fmt.Println("Refresh errored ", err)

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
