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
		"NODE_VERSION": "18",
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

	// install fly-apps/dockerfile-laravel if it's not already installed
	if !installed {
		args := []string{"composer", "require", "--dev", "fly-apps/dockerfile-laravel"}
		fmt.Printf("installing: %s\n", strings.Join(args, " "))
		cmd := exec.Command(args[0], args[1:]...)

		if err := runGenerator(cmd, !srcInfo.NonInteractive); err != nil {
			switch {
			case dockerfileExists:
				// The existing Dockerfile will do; the generator only adds to it.
				fmt.Printf("Couldn't install fly-apps/dockerfile-laravel; using your existing Dockerfile.\n")
			case errors.Is(err, exec.ErrNotFound):
				return fmt.Errorf("fly launch needs Composer to generate a Dockerfile for a Laravel app, and composer isn't on your PATH. Install PHP and Composer (https://getcomposer.org), or add a Dockerfile, and run fly launch again")
			default:
				return fmt.Errorf("Dockerfile doesn't exist and failed to install fly-apps/dockerfile-laravel: %w", err)
			}
		}
	}

	// The install above can fail with a Dockerfile already present; there is
	// then no generator to run.
	if _, err := os.Stat(vendorPath); err == nil {
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

		if err := runGenerator(cmd, !srcInfo.NonInteractive); err != nil {
			return fmt.Errorf("failed to generate Dockerfile: %w", err)
		}
	} else if !dockerfileExists {
		return fmt.Errorf("failed to generate Dockerfile: %s is missing; run composer install and fly launch again", vendorPath)
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

	version := ""
	if len(match) > 1 {
		version = match[1]
	}

	// composer.json gives the oldest PHP the app allows, but composer.lock may
	// pin packages that need a newer one (a lock resolved on PHP 8.4 can pin
	// symfony/console 8, which needs PHP 8.4). Composer can't install anything
	// into that lock on an older PHP, so use the newest floor of the two.
	if locked := lockedPhpFloor("composer.lock"); locked != "" && (version == "" || phpVersionLess(version, locked)) {
		version = locked
	}

	if version != "" {
		return version, nil
	}

	return "", fmt.Errorf("could not find php version")
}

var phpMajorMinor = regexp.MustCompile(`([0-9]+)\.([0-9]+)`)

// lockedPhpFloor returns the oldest PHP major.minor that every package in a
// composer.lock accepts, or "" when the lock is missing or names no PHP floor.
func lockedPhpFloor(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var lock struct {
		Packages    []struct{ Require map[string]string } `json:"packages"`
		PackagesDev []struct{ Require map[string]string } `json:"packages-dev"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		return ""
	}

	floor := ""
	for _, pkg := range append(lock.Packages, lock.PackagesDev...) {
		if v := phpConstraintFloor(pkg.Require["php"]); v != "" && (floor == "" || phpVersionLess(floor, v)) {
			floor = v
		}
	}

	return floor
}

// phpConstraintFloor returns the oldest PHP major.minor a Composer constraint
// accepts: "8.4" for ">=8.4.1", "7.4" for "^7.4|^8.0". It returns "" when the
// constraint, or any of its alternatives, has no lower bound.
func phpConstraintFloor(constraint string) string {
	floor := ""
	for _, alternative := range strings.Split(strings.ReplaceAll(constraint, "||", "|"), "|") {
		lower := ""
		for _, term := range strings.FieldsFunc(alternative, func(r rune) bool { return r == ' ' || r == ',' }) {
			if strings.HasPrefix(term, "<") || strings.HasPrefix(term, "!=") {
				continue
			}
			if m := phpMajorMinor.FindString(term); m != "" {
				lower = m
				break
			}
		}
		if lower == "" {
			return ""
		}
		if floor == "" || phpVersionLess(lower, floor) {
			floor = lower
		}
	}

	return floor
}

// phpVersionLess compares two major.minor versions.
func phpVersionLess(a, b string) bool {
	am, bm := phpMajorMinor.FindStringSubmatch(a), phpMajorMinor.FindStringSubmatch(b)
	if am == nil || bm == nil {
		return false
	}
	aMajor, _ := strconv.Atoi(am[1])
	aMinor, _ := strconv.Atoi(am[2])
	bMajor, _ := strconv.Atoi(bm[1])
	bMinor, _ := strconv.Atoi(bm[2])

	return aMajor < bMajor || (aMajor == bMajor && aMinor < bMinor)
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
