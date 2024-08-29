package scanner

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command/launch/plan"
)

// setup Laravel with a sqlite database
func configureLaravel(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// Laravel projects contain the `artisan` command
	if !checksPass(sourceDir, fileExists("artisan")) {
		return nil, nil
	}

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

	// Min PHP version to use generator
	minVersion, err := semver.Make("8.1.0")
	if err != nil {
		panic(err)
	}

	// The detected PHP version
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

	// Use default scanner templates if < min version(8.1.0)
	phpNVersion, err := semver.Make(phpVersion + ".0")
	if err != nil || phpNVersion.LT(minVersion) {
		s.Files = templates("templates/laravel")
	} else {
		// Else use dockerfile-laravel generator
		s.Callback = LaravelCallback
	}

	// Extract DB, Redis config from dotenv
	db, redis, skipDB := extractConnections(".env")
	s.SkipDatabase = skipDB
	s.RedisDesired = redis
	if db != 0 {
		s.DatabaseDesired = db
	}

	// Enable Object Storage( Tigris ) when
	// * league/flysystem-aws-s3* found in composer.json
	if checksPass(sourceDir, dirContains("composer.json", "league/flysystem-aws-s3")) {
		s.ObjectStorageDesired = true
		s.OverrideExtensionSecretKeyNames = make(map[string]map[string]string)
		s.OverrideExtensionSecretKeyNames["tigris"] = make(map[string]string)

		// Replace the following secret key names set from the tigris extension provisioning
		// With their custom secret key name vales
		s.OverrideExtensionSecretKeyNames["tigris"]["AWS_REGION"] = "AWS_DEFAULT_REGION"
		s.OverrideExtensionSecretKeyNames["tigris"]["BUCKET_NAME"] = "AWS_BUCKET"
		s.OverrideExtensionSecretKeyNames["tigris"]["AWS_ENDPOINT_URL_S3"] = "AWS_ENDPOINT"
	}

	return s, nil
}

func LaravelCallback(appName string, srcInfo *SourceInfo, plan *plan.LaunchPlan, flags []string) error {
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
	dockerfileExists := true
	_, err = os.Stat("Dockerfile")
	if errors.Is(err, fs.ErrNotExist) {
		dockerfileExists = false
	}

	// check first to see if the package is already installed
	installed := false

	data, err := os.ReadFile("composer.json")
	if err == nil {
		var composerJson map[string]interface{}
		err = json.Unmarshal(data, &composerJson)
		if err == nil {
			// check for the package in the composer.json
			require, ok := composerJson["require"].(map[string]interface{})
			if ok && require["fly-apps/dockerfile-laravel"] != nil {
				installed = true
			}

			requireDev, ok := composerJson["require-dev"].(map[string]interface{})
			if ok && requireDev["fly-apps/dockerfile-laravel"] != nil {
				installed = true
			}
		}
	}

	// check if executable is available
	vendorPath := filepath.Join("vendor", "bin", "dockerfile-laravel")
	_, err = os.Stat(vendorPath)
	if os.IsNotExist(err) {
		installed = false
	}

	// install fly-apps/dockerfile-laravel if it's not already installed
	if !installed {
		args := []string{"composer", "require", "--dev", "fly-apps/dockerfile-laravel"}
		fmt.Printf("installing: %s\n", strings.Join(args, " "))
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil && !dockerfileExists {
			return fmt.Errorf("Dockerfile doesn't exist and failed to install fly-apps/dockerfile-laravel: %w", err)
		}
	}

	args := []string{vendorPath, "generate"}
	if dockerfileExists {
		args = append(args, "--skip")
	}

	// add additional flags from launch command
	if len(flags) > 0 {
		args = append(args, flags...)
	}

	fmt.Printf("Running: %s\n", strings.Join(args, " "))
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to generate Dockerfile: %w", err)
	}

	// provide some advice
	srcInfo.DeployDocs += fmt.Sprintf(`
If you need custom packages installed, or have problems with your deployment
build, you may need to edit the Dockerfile for app-specific changes. If you
need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your %s app.
`, srcInfo.Family)

	return nil
}

func extractPhpVersion() (string, error) {
	/* VIA composer.json file */
	// Capture major/minor version (leaving out revision version)
	re := regexp.MustCompile(`([0-9]+\.[0-9]+)`)
	var match = re.FindStringSubmatch("")

	data, err := os.ReadFile("composer.json")
	if err == nil {
		var composerJson map[string]interface{}
		err = json.Unmarshal(data, &composerJson)
		if err == nil {
			// check for the package in the composer.json
			require, ok := composerJson["require"].(map[string]interface{})
			if ok && require["php"] != nil {
				str := fmt.Sprint(require["php"])
				match = re.FindStringSubmatch(str)
			}
		}
	}

	if len(match) == 0 {
		/* VIA php artisan version:
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
		match = re.FindStringSubmatch(string(out))
	}

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

var dbRegStr = "^ *(DB_CONNECTION|DATABASE_URL) *= *(\"|')? *[a-zA-Z]+(\"|')?"
var redisRegStr = "^[^#]*redis"

// extractConnections detects the database connection of a laravel fly app
// by checking the .env file in the project's base directory for connection keywords.
// This ignores commented out lines and prioritizes the first connection occurance over others.
//
// Returns three variables:
//
//	db - DatabaseKind of the connection extracted
//	redis - reports whether redis was detected
//	skipDb - reports whether a connection or redis was detected
func extractConnections(path string) (db DatabaseKind, redis bool, skipDb bool) {
	// Get File Content

	file, err := os.Open(path)
	if err != nil {
		return 0, false, true
	}
	defer file.Close() //skipcq: GO-S2307

	// Set up Regex to match
	// -not commented out, with DB_CONNECTION
	dbReg := regexp.MustCompile(dbRegStr)
	// -not commented out with redis keyword
	redisReg := regexp.MustCompile(redisRegStr)

	// Default Return Variables
	db = 0
	redis = false
	skipDb = true

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
			} else if strings.Contains(text, "pgsql") || strings.Contains(text, "postgres") {
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
