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
	"runtime"
	"slices"
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

	// Oldest PHP the dockerfile-laravel generator supports
	minVersion, err := semver.Make("8.1.0")
	if err != nil {
		panic(err)
	}

	// The detected PHP version
	phpVersion, err := extractPhpVersion()
	if err != nil || phpVersion == "" {
		phpVersion = "8.1"
	}

	phpNVersion, err := semver.Make(phpVersion + ".0")
	if err != nil || phpNVersion.LT(minVersion) {
		return nil, fmt.Errorf("Laravel apps on Fly.io need PHP 8.1 or newer; detected PHP %s. Update the php requirement in composer.json and run fly launch again", phpVersion)
	}

	s.Runtime = plan.RuntimeStruct{Language: "php", Version: phpVersion}

	s.BuildArgs = map[string]string{
		"PHP_VERSION":  phpVersion,
		"NODE_VERSION": "22",
	}

	// The Dockerfile comes from the dockerfile-laravel generator
	s.Callback = LaravelCallback

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
		var composerJson map[string]any
		err = json.Unmarshal(data, &composerJson)
		if err == nil {
			// check for the package in the composer.json
			require, ok := composerJson["require"].(map[string]any)
			if ok && require["fly-apps/dockerfile-laravel"] != nil {
				installed = true
			}

			requireDev, ok := composerJson["require-dev"].(map[string]any)
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

	// the generator is a PHP script; running it through php works on every
	// platform, including Windows, where vendor/bin/dockerfile-laravel isn't
	// directly executable
	_, phpErr := exec.LookPath("php")
	_, composerErr := exec.LookPath("composer")

	// install fly-apps/dockerfile-laravel if it's not already installed
	var installErr error
	switch {
	case installed && phpErr != nil:
		installErr = errors.New("php was not found on your PATH")
	case !installed && (phpErr != nil || composerErr != nil):
		installErr = errors.New("php or composer was not found on your PATH")
	case !installed:
		installErr = runCommand("composer", "require", "--dev", "fly-apps/dockerfile-laravel")
	}

	generateArgs := []string{"generate"}
	if dockerfileExists {
		generateArgs = append(generateArgs, "--skip")
	}

	// add additional flags from launch command
	if len(flags) > 0 {
		generateArgs = append(generateArgs, flags...)
	}

	switch {
	case installErr == nil:
		if err := runCommand("php", append([]string{vendorPath}, generateArgs...)...); err != nil {
			return fmt.Errorf("failed to generate Dockerfile: %w", err)
		}
	case dockerfileExists:
		// the generator only adds supporting files around an existing
		// Dockerfile, so the launch can go ahead without it
		fmt.Printf("Skipping fly-apps/dockerfile-laravel (%v); using your existing Dockerfile\n", installErr)
	case phpErr == nil && composerErr == nil:
		return fmt.Errorf("Dockerfile doesn't exist and failed to install fly-apps/dockerfile-laravel: %w", installErr)
	default:
		if _, err := exec.LookPath("docker"); err != nil {
			return fmt.Errorf("fly launch needs PHP and Composer, or Docker, to generate a Dockerfile for your Laravel app (%w). Install Composer (https://getcomposer.org) and run fly launch again", installErr)
		}

		if err := generateLaravelDockerfileInDocker(generateArgs); err != nil {
			return fmt.Errorf("failed to generate Dockerfile: %w", err)
		}
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

// generateLaravelDockerfileInDocker installs and runs fly-apps/dockerfile-laravel
// in the official Composer image, for machines without PHP or Composer.
func generateLaravelDockerfileInDocker(generateArgs []string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}

	docker := []string{"run", "--rm", "-i", "-v", dir + ":/app", "-w", "/app", "-e", "COMPOSER_HOME=/tmp/composer"}
	if runtime.GOOS != "windows" {
		// keep the files the generator writes owned by the current user
		docker = append(docker, "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()))
	}
	docker = append(docker, "composer:2")

	// the image's PHP may not match the app's PHP version or extensions,
	// which only matter at runtime, not for installing the generator
	err = runCommand("docker", slices.Concat(docker, []string{"composer", "require", "--dev", "--no-interaction", "--ignore-platform-reqs", "fly-apps/dockerfile-laravel"})...)
	if err != nil {
		return err
	}

	return runCommand("docker", slices.Concat(docker, []string{"php", "vendor/bin/dockerfile-laravel"}, generateArgs)...)
}

func runCommand(name string, args ...string) error {
	fmt.Printf("Running: %s %s\n", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func extractPhpVersion() (string, error) {
	/* VIA composer.json file */
	// Capture major/minor version (leaving out revision version)
	re := regexp.MustCompile(`([0-9]+\.[0-9]+)`)
	var match = re.FindStringSubmatch("")

	data, err := os.ReadFile("composer.json")
	if err == nil {
		var composerJson map[string]any
		err = json.Unmarshal(data, &composerJson)
		if err == nil {
			// check for the package in the composer.json
			require, ok := composerJson["require"].(map[string]any)
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
		return match[1], nil
	}

	return "", fmt.Errorf("could not find php version")
}

var dbRegStr = "^ *(DB_CONNECTION|DATABASE_URL) *= *(\"|')? *[a-zA-Z]+(\"|')?"
var redisRegStr = "^[^#]*redis"

// extractConnections detects the database connection of a laravel fly app
// by checking the .env file in the project's base directory for connection keywords.
// This ignores commented out lines and prioritizes the first connection occurrence over others.
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
