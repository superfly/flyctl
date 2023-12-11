package scanner

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"github.com/superfly/flyctl/helpers"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

type ComposerLock struct {
	Platform PhpVersion `json:"platform,omitempty"`
}

type PhpVersion struct {
	Version string `json:"php"`
}

// setup Laravel with a sqlite database
func configureLaravel(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// Laravel projects contain the `artisan` command
	if !checksPass(sourceDir, fileExists("artisan")) {
		return nil, nil
	}

	files := templates("templates/laravel")

	s := &SourceInfo{
		Env: map[string]string{
			"APP_ENV":               "production",
			"LOG_CHANNEL":           "stderr",
			"LOG_LEVEL":             "info",
			"LOG_STDERR_FORMATTER":  "Monolog\\Formatter\\JsonFormatter",
			"SESSION_DRIVER":        "cookie",
			"SESSION_SECURE_COOKIE": "true",
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
		SkipDatabase:   true,
		ConsoleCommand: "php /var/www/html/artisan tinker",
	}

	phpVersion, err := extractPhpVersion()

	if err != nil || phpVersion == "" {
		// Fallback to 8.0, which has
		// the broadest compatibility
		phpVersion = "8.0"
	}

	s.BuildArgs = map[string]string{
		"PHP_VERSION":  phpVersion,
		"NODE_VERSION": "18",
	}

	// Extract DB, Redis config from dotenv
	db, redis, skipDB := extractConnections(".env")
	s.SkipDatabase = skipDB
	s.RedisDesired = redis
	if db != 0 {
		s.DatabaseDesired = db
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

func extractConnections(path string) (DatabaseKind, bool, bool) {
	/*
		Determine the default db
		Determine whether redis connection is desired
			returns ( db, redis, skipDB )
	*/

	// Get File Content
	file, err := os.Open(path)
	if err != nil {
		return 0, false, true
	}
	defer file.Close() //skipcq: GO-S2307

	// Set up Regex to match
	// -not commented out, with DB_CONNECTION
	dbReg := regexp.MustCompile("^ *DB_CONNECTION *= *[a-zA-Z]+")
	// -not commented out with redis keyword
	redisReg := regexp.MustCompile("^[^#]*redis")

	// Default Return Variables
	var db DatabaseKind = 0
	redis := false
	skipDb := true

	// Check each line for
	// match on redis or db regex
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()

		if redisReg.MatchString(text) {
			redis = true
			skipDb = false
		} else if db == 0 && dbReg.MatchString(text) {
			if strings.Contains(text, "mysql") {
				db = DatabaseKindMySQL
				skipDb = false
			} else if strings.Contains(text, "pgsql") {
				db = DatabaseKindPostgres
				skipDb = false
			} else if strings.Contains(text, "sqlite") {
				db = DatabaseKindSqlite
				skipDb = false
			}
		}
	}

	return db, redis, skipDb
}
