package scanner

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"

	"github.com/superfly/flyctl/helpers"
)

type ComposerLock struct {
	Platform PhpVersion `json:"platform,omitempty"`
}

type PhpVersion struct {
	Version string `json:"php"`
}

// setup Laravel with a sqlite database
func configureLaravel(sourceDir string) (*SourceInfo, error) {
	// Laravel projects contain the `artisan` command
	if !checksPass(sourceDir, fileExists("artisan")) {
		return nil, nil
	}

	files := templates("templates/laravel")

	s := &SourceInfo{
		Env: map[string]string{
			"APP_ENV":              "production",
			"LOG_CHANNEL":          "stderr",
			"LOG_LEVEL":            "info",
			"LOG_STDERR_FORMATTER": "Monolog\\Formatter\\JsonFormatter",
		},
		Family: "Laravel",
		Files:  files,
		Port:   8080,
		Secrets: []Secret{
			{
				Key:  "APP_KEY",
				Help: "Laravel needs a unique application key.",
				Generate: func() (string, error) {
					// Method used in RandBytes never returns an error
					r, _ := helpers.RandBytes(32)
					return "base64:" + base64.StdEncoding.EncodeToString(r), nil
				},
			},
		},
		SkipDatabase: true,
	}

	phpVersion, err := extractPhpVersion()

	if err != nil || phpVersion == "" {
		// Fallback to 8.0, which has
		// the broadest compatibility
		phpVersion = "8.0"
	}

	s.BuildArgs = map[string]string{
		"PHP_VERSION":  phpVersion,
		"NODE_VERSION": "14",
	}

	return s, nil
}

func extractPhpVersion() (string, error) {
	/* Example Output:
	PHP 8.1.8 (cli) (built: Jul  8 2022 10:58:31) (NTS)
	Copyright (c) The PHP Group
	Zend Engine v4.1.8, Copyright (c) Zend Technologies
		with Zend OPcache v8.1.8, Copyright (c), by Zend Technologies
	*/
	cmd := exec.Command("php", "-v")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	// Capture major/minor version (leaving out revision version)
	re := regexp.MustCompile(`PHP ([0-9]+\.[0-9]+)\.[0-9]`)
	match := re.FindStringSubmatch(string(out))

	if len(match) > 1 {
		// If the PHP version is below 7.4, we won't have a
		// container for it, so we'll use PHP 7.4
		if match[1][0:1] == "7" {
			vers, err := strconv.ParseFloat(match[1], 32)
			if err != nil {
				return "7.4", nil
			}
			if vers < 7.4 {
				return "7.4", nil
			}
		}
		return match[1], nil
	}

	return "", fmt.Errorf("could not find php version")
}
