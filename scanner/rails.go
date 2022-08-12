package scanner

import (
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
)

func configureRails(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("Gemfile", "rails")) {
		return nil, nil
	}

	s := &SourceInfo{
		Files:  templates("templates/rails/standard"),
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
		ReleaseCmd: "bundle exec rails db:migrate",
		Env: map[string]string{
			"SERVER_COMMAND": "bundle exec puma -C config/puma.rb",
			"PORT":           "8080",
		},
	}

	var rubyVersion string
	var bundlerVersion string
	var nodeVersion string = "14"

	rubyVersion, err := extractRubyVersion("Gemfile", ".ruby_version")

	if err != nil || rubyVersion == "" {
		rubyVersion = "3.1.1"
	}

	bundlerVersion, err = extractBundlerVersion("Gemfile.lock")

	if err != nil || bundlerVersion == "" {
		bundlerVersion = "2.3.9"
	}

	s.BuildArgs = map[string]string{
		"RUBY_VERSION":    rubyVersion,
		"BUNDLER_VERSION": bundlerVersion,
		"NODE_VERSION":    nodeVersion,
	}

	// master.key comes with Rails apps from v6 onwards, but may not be present
	// if the app does not use Rails encrypted credentials
	masterKey, err := ioutil.ReadFile("config/master.key")

	if err == nil {
		s.Secrets = []Secret{
			{
				Key:   "RAILS_MASTER_KEY",
				Help:  "Secret key for accessing encrypted credentials",
				Value: string(masterKey),
			},
		}
	}

	s.SkipDeploy = true
	s.DeployDocs = fmt.Sprintf(`
Your Rails app is prepared for deployment. Production will be setup with these versions of core runtime packages:

Ruby %s
Bundler %s
NodeJS %s

You can configure these in the [build] section in the generated fly.toml.

Ruby versions available are: 3.1.1, 3.0.3, 2.7.5, and 2.6.9. Learn more about the chosen Ruby stack, Fullstaq Ruby, here: https://github.com/evilmartians/fullstaq-ruby-docker.
We recommend using the highest patch level for better security and performance.

For the other packages, specify any version you need.

If you need custom packages installed, or have problems with your deployment build, you may need to edit the Dockerfile
for app-specific changes. If you need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your Rails app.
`, rubyVersion, bundlerVersion, nodeVersion)

	return s, nil
}

func extractRubyVersion(gemfilePath string, rubyVersionPath string) (string, error) {
	gemfileContents, err := os.ReadFile(gemfilePath)

	var version string

	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`ruby \"(?P<version>.+)\"`)
	m := re.FindStringSubmatch(string(gemfileContents))

	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "version" {
			version = m[i]
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

	re := regexp.MustCompile(`BUNDLED WITH\n\s{3}(?P<version>.+)\n`)
	m := re.FindStringSubmatch(string(gemfileContents))
	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "version" {
			version = m[i]
		}
	}

	return version, nil
}
