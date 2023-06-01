package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/blang/semver"
)

var packageJson map[string]interface{}

// Handle node frameworks separate from other node applications.  Currently the requirements
// for a framework is pretty low: to have a "start" script.  Because we are actually
// going to be running a node application to generate a Dockerfile there is one more
// criteria: the running node version must be at least 16.  If there turns out to be
// demand for earlier versions of node, we can adjust this requirement.
func configureNodeFramework(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// first ensure that there is a package.json
	if !checksPass(sourceDir, fileExists("package.json")) {
		return nil, nil
	}

	// ensure package.json has a start script
	data, err := os.ReadFile("package.json")

	if err != nil {
		return nil, nil
	} else {
		err = json.Unmarshal(data, &packageJson)
		if err != nil {
			return nil, nil
		}

		scripts, ok := packageJson["scripts"].(map[string]interface{})

		if !ok || scripts["start"] == nil {
			return nil, nil
		}
	}

	// ensure node version is at least 16.0.0
	out, err := exec.Command("node", "-v").Output()
	if err != nil {
		return nil, nil
	} else {
		minVersion, err := semver.Make("16.0.0")
		if err != nil {
			panic(err)
		}

		nodeVersionString := strings.TrimSpace(string(out))
		if nodeVersionString[:1] == "v" {
			nodeVersionString = nodeVersionString[1:]
		}

		nodeVersion, err := semver.Make(nodeVersionString)

		if err != nil || nodeVersion.LT(minVersion) {
			return nil, nil
		}
	}

	srcInfo := &SourceInfo{
		Family:     "NodeJS",
		SkipDeploy: true,
		Callback:   NodeFrameworkCallback,
	}

	// don't prompt for redis or postgres unless they are used
	deps, ok := packageJson["dependencies"].(map[string]interface{})
	if !ok || (deps["pg"] == nil && deps["redis"] == nil) {
		srcInfo.SkipDatabase = true
	}

	return srcInfo, nil
}

func NodeFrameworkCallback(appName string, srcInfo *SourceInfo, options map[string]bool) error {
	// create temporary fly.toml for merge purposes
	flyToml := "fly.toml"
	_, err := os.Stat(flyToml)
	if os.IsNotExist(err) {
		// create a fly.toml consisting only of an app name
		contents := fmt.Sprintf("app = \"%s\"\n", appName)
		err := os.WriteFile(flyToml, []byte(contents), 0644)
		if err != nil {
			log.Fatal(err)
		}

		// inform caller of the presence of this file
		srcInfo.MergeConfig = &MergeConfigStruct{
			Name:      flyToml,
			Temporary: true,
		}
	}

	// generate Dockerfile if it doesn't already exist
	_, err = os.Stat("Dockerfile")
	if errors.Is(err, fs.ErrNotExist) {
		var args []string

		_, err = os.Stat("node_modules")
		if errors.Is(err, fs.ErrNotExist) {
			// no existing node_modules directory: run package directly
			args = []string{"npx", "--yes", "@flydotio/dockerfile@latest"}
		} else {
			// build command to install package using preferred package manager
			args = []string{"npm", "install", "@flydotio/dockerfile", "--save-dev"}

			_, err = os.Stat("yarn.lock")
			if !errors.Is(err, fs.ErrNotExist) {
				args = []string{"yarn", "add", "@flydotio/dockerfile", "--dev"}
			}

			_, err = os.Stat("pnpm-lock.yaml")
			if !errors.Is(err, fs.ErrNotExist) {
				args = []string{"pnpm", "add", "-D", "@flydotio/dockerfile"}
			}
		}

		// check first to see if the package is already installed
		installed := false

		deps, ok := packageJson["dependencies"].(map[string]interface{})
		if ok && deps["@flydotio/dockerfile"] != nil {
			installed = true
		}

		deps, ok = packageJson["devDependencies"].(map[string]interface{})
		if ok && deps["@flydotio/dockerfile"] != nil {
			installed = true
		}

		// install/run command
		if !installed || args[0] == "npx" {
			fmt.Printf("installing: %s\n", strings.Join(args[:], " "))
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Stdin = nil
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to install @flydotio/dockerfile: %w", err)
			}
		}

		// run the package if we haven't already
		if args[0] != "npx" {
			cmd := exec.Command("npx", "dockerfile")
			cmd.Stdin = nil
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to generate Dockerfile: %w", err)
			}
		}
	}

	// read dockerfile
	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		return err
	}

	// extract family
	family := "NodeJS"
	re := regexp.MustCompile(`(?m)^LABEL\s+fly_launch_runtime="(?P<family>.+?)"`)
	m := re.FindStringSubmatch(string(dockerfile))

	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "family" {
			family = m[i]
		}
	}
	srcInfo.Family = family

	// extract port
	port := 3000
	re = regexp.MustCompile(`(?m)^EXPOSE\s+(?P<port>\d+)`)
	m = re.FindStringSubmatch(string(dockerfile))

	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "port" {
			port, err = strconv.Atoi(m[i])
			if err != nil {
				panic(err)
			}
		}
	}
	srcInfo.Port = port

	// provide some advice
	srcInfo.DeployDocs += `
If you need custom packages installed, or have problems with your deployment
build, you may need to edit the Dockerfile for app-specific changes. If you
need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your Node app.
`

	return nil
}
