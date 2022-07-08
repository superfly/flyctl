package scanner

import (
	"encoding/base64"

	"github.com/superfly/flyctl/helpers"
)

// setup Laravel with a sqlite database
func configureLaravel(sourceDir string) (*SourceInfo, error) {
	// Laravel projects contain the `artisan` command
	if !checksPass(sourceDir, fileExists("artisan")) {
		return nil, nil
	}

	files := templates("templates/laravel/common")

	var extra []SourceFile
	if checksPass(sourceDir, dirContains("composer.json", "laravel/octane")) {
		extra = templates("templates/laravel/octane")
	} else {
		extra = templates("templates/laravel/standard")
	}

	// Merge common files with runtime-specific files (standard or octane)
	for _, f := range extra {
		files = append(files, f)
	}

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

	return s, nil
}
