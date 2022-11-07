package scanner

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

func configureRails(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("Gemfile", "rails")) {
		return nil, nil
	}

	vars := make(map[string]interface{})

	s := &SourceInfo{
		Family: "Rails",
		Port:   8080,
		Statics: []Static{
			{
				GuestPath: "/app/public",
				UrlPrefix: "/",
			},
		},
		PostgresInitCommands: []InitCommand{
			{
				Command:     "bundle",
				Args:        []string{"add", "pg"},
				Description: "Adding the 'pg' gem for Postgres database support",
				Condition:   !checksPass(sourceDir, dirContains("Gemfile", "pg")),
			},
		},
		ReleaseCmd: "bin/rails fly:release",
		Env: map[string]string{
			"PORT": "8080",
		},
		BuildArgs: map[string]string{
			"BUILD_COMMAND":  "bin/rails fly:build",
			"SERVER_COMMAND": "bin/rails fly:server",
		},
	}

	var rubyVersion string
	var bundlerVersion string
	var nodeVersion string = "latest"
	var yarnVersion string = "latest"

	out, err := exec.Command("node", "-v").Output()

	if err == nil {
		nodeVersion = strings.TrimSpace(string(out))
		if nodeVersion[:1] == "v" {
			nodeVersion = nodeVersion[1:]
		}
	}

	out, err = exec.Command("yarn", "-v").Output()

	if err == nil {
		yarnVersion = strings.TrimSpace(string(out))
	}

	rubyVersion, err = extractRubyVersion("Gemfile.lock", "Gemfile", ".ruby_version")

	if err != nil || rubyVersion == "" {
		rubyVersion = "3.1.2"

		out, err := exec.Command("ruby", "-v").Output()
		if err == nil {

			version := strings.TrimSpace(string(out))
			re := regexp.MustCompile(`ruby (?P<version>[\d.]+)`)
			m := re.FindStringSubmatch(version)

			for i, name := range re.SubexpNames() {
				if len(m) > 0 && name == "version" {
					rubyVersion = m[i]
				}
			}
		}
	}

	bundlerVersion, err = extractBundlerVersion("Gemfile.lock")

	if err != nil || bundlerVersion == "" {
		bundlerVersion = "2.3.21"

		out, err := exec.Command("bundle", "-v").Output()
		if err == nil {

			version := strings.TrimSpace(string(out))
			re := regexp.MustCompile(`Bundler version (?P<version>[\d.]+)`)
			m := re.FindStringSubmatch(version)

			for i, name := range re.SubexpNames() {
				if len(m) > 0 && name == "version" {
					bundlerVersion = m[i]
				}
			}
		}
	}

	// master.key comes with Rails apps from v5.2 onwards, but may not be present
	// if the app does not use Rails encrypted credentials.  Rails v6 added
	// support for multi-environment credentials.  Use the Rails searching
	// sequence for production credentials to determine the RAILS_MASTER_KEY.
	masterKey, err := os.ReadFile("config/credentials/production.key")
	if err != nil {
		masterKey, err = os.ReadFile("config/master.key")
	}

	if err == nil {
		s.Secrets = []Secret{
			{
				Key:   "RAILS_MASTER_KEY",
				Help:  "Secret key for accessing encrypted credentials",
				Value: string(masterKey),
			},
		}
	}

	_, err = os.Stat("node_modules")
	vars["node"] = !os.IsNotExist(err)

	_, err = os.Stat("yarn.lock")
	vars["yarn"] = !os.IsNotExist(err)

	vars["rubyVersion"] = rubyVersion
	vars["bundlerVersion"] = bundlerVersion
	vars["nodeVersion"] = nodeVersion
	vars["yarnVersion"] = yarnVersion
	s.Files = templatesExecute("templates/rails/standard", vars)

	s.SkipDeploy = true
	s.DeployDocs = `
Your Rails app is prepared for deployment.

If you need custom packages installed, or have problems with your deployment
build, you may need to edit the Dockerfile for app-specific changes. If you
need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your Rails app.
`

	return s, nil
}

func extractRubyVersion(lockfilePath string, gemfilePath string, rubyVersionPath string) (string, error) {

	var version string

	lockfileContents, err := os.ReadFile(lockfilePath)

	if err == nil {
		re := regexp.MustCompile(`RUBY VERSION\s+ruby (?P<version>[\d.]+)`)
		m := re.FindStringSubmatch(string(lockfileContents))

		for i, name := range re.SubexpNames() {
			if len(m) > 0 && name == "version" {
				version = m[i]
			}
		}
	}

	if version == "" {
		gemfileContents, err := os.ReadFile(gemfilePath)

		if err != nil {
			return "", err
		}

		re := regexp.MustCompile(`ruby \"(?P<version>[\d.]+)\"`)
		m := re.FindStringSubmatch(string(gemfileContents))

		for i, name := range re.SubexpNames() {
			if len(m) > 0 && name == "version" {
				version = m[i]
			}
		}
	}

	if version == "" {
		if _, err := os.Stat(rubyVersionPath); err == nil {

			versionString, err := os.ReadFile(rubyVersionPath)
			if err != nil {
				return "", err
			}

			version = string(versionString)
		}
	}

	return version, nil
}

func extractBundlerVersion(gemfileLockPath string) (string, error) {
	gemfileContents, err := os.ReadFile(gemfileLockPath)

	var version string

	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`BUNDLED WITH\n\s{3}(?P<version>[\d.]+)\n`)
	m := re.FindStringSubmatch(string(gemfileContents))
	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "version" {
			version = m[i]
		}
	}

	return version, nil
}
