package scanner

import (
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/pkg/errors"
)

func configureRuby(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Gemfile", "config.ru")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Ruby",
		Port:   8080,
	}

	rubyVersion, err := extractRubyVersion("Gemfile.lock", "Gemfile", ".ruby_version")
	if err != nil {
		return nil, errors.Wrap(err, "failure extracting Ruby version")
	}

	vars := make(map[string]interface{})
	vars["rubyVersion"] = rubyVersion
	s.Files = templatesExecute("templates/ruby", vars)

	s.SkipDeploy = true
	s.DeployDocs = `
Your Ruby app is prepared for deployment.

If you need custom packages installed, or have problems with your deployment
build, you may need to edit the Dockerfile for app-specific changes. If you
need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your Ruby app.
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

	if version == "" {
		version = "3.3.5"

		out, err := exec.Command("ruby", "-v").Output()
		if err == nil {

			versionString := strings.TrimSpace(string(out))
			re := regexp.MustCompile(`ruby (?P<version>[\d.]+)`)
			m := re.FindStringSubmatch(versionString)

			for i, name := range re.SubexpNames() {
				if len(m) > 0 && name == "version" {
					version = m[i]
				}
			}
		}
	}

	return version, nil
}
