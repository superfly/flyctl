package scanner

import (
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/internal/command/launch/plan"
)

var healthcheck_channel = make(chan string)
var bundle, ruby string

func configureRails(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// `bundle init` will create a file with a commented out rails gem,
	// so checking for that can produce a false positive.  Look for
	// Rails three other ways...
	rails := checksPass(sourceDir+"/bin", fileExists("rails")) ||
		checksPass(sourceDir, dirContains("config.ru", "Rails")) ||
		checksPass(sourceDir, dirContains("Gemfile.lock", " rails "))

	if !rails {
		return nil, nil
	}

	// find absolute pat to bundle, ruby executables
	// see: https://tip.golang.org/doc/go1.19#os-exec-path
	var err error
	bundle, err = exec.LookPath("bundle")
	if err != nil {
		if errors.Is(err, exec.ErrDot) {
			bundle, err = filepath.Abs(bundle)
		}

		if err != nil {
			return nil, errors.Wrap(err, "failure finding bundle executable")
		}
	}

	ruby, err = exec.LookPath("ruby")
	if err != nil {
		if errors.Is(err, exec.ErrDot) {
			ruby, err = filepath.Abs(ruby)
		}

		if err != nil {
			return nil, errors.Wrap(err, "failure finding ruby executable")
		}
	}

	// verify that the bundle will install before proceeding
	args := []string{"install"}

	if checksPass(sourceDir, fileExists("Gemfile.lock")) {
		args = append(args, "--quiet")
	}

	cmd := exec.Command(bundle, args...)
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, errors.Wrap(err, "Failed to install bundle, exiting")
	}

	s := &SourceInfo{
		Family:               "Rails",
		Callback:             RailsCallback,
		ConsoleCommand:       "/rails/bin/rails console",
		AutoInstrumentErrors: true,
	}

	// don't prompt for pg, redis if litestack is in the Gemfile
	if checksPass(sourceDir, dirContains("Gemfile", "litestack")) {
		s.SkipDatabase = true
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
	} else {
		// find absolute path to rake executable
		rake, err := exec.LookPath("rake")
		if err != nil {
			if errors.Is(err, exec.ErrDot) {
				rake, err = filepath.Abs(rake)
			}

			if err != nil {
				return nil, errors.Wrap(err, "failure finding rake executable")
			}
		}

		// support Rails 4 through 5.1 applications, or ones that started out
		// there and never were fully upgraded.
		out, err := exec.Command(rake, "secret").Output()

		if err == nil {
			s.Secrets = []Secret{
				{
					Key:   "SECRET_KEY_BASE",
					Help:  "Secret key used to verify the integrity of signed cookies",
					Value: strings.TrimSpace(string(out)),
				},
			}
		}
	}

	s.SkipDeploy = true
	s.DeployDocs = `
Your Rails app is prepared for deployment.

Before proceeding, please review the posted Rails FAQ:
https://fly.io/docs/rails/getting-started/dockerfiles/.

Once ready: run 'fly deploy' to deploy your Rails app.
`

	// fetch healthcheck route in a separate thread
	go func() {
		ruby, err := exec.LookPath("ruby")
		if err != nil {
			healthcheck_channel <- ""
			return
		}

		out, err := exec.Command(ruby, "./bin/rails", "runner",
			"puts Rails.application.routes.url_helpers.rails_health_check_path").Output()

		if err == nil {
			healthcheck_channel <- strings.TrimSpace(string(out))
		} else {
			healthcheck_channel <- ""
		}
	}()

	return s, nil
}

func RailsCallback(appName string, srcInfo *SourceInfo, plan *plan.LaunchPlan) error {
	// install dockerfile-rails gem, if not already included
	gemfile, err := os.ReadFile("Gemfile")
	if err != nil {
		panic(err)
	} else if !strings.Contains(string(gemfile), "dockerfile-rails") {
		cmd := exec.Command(bundle, "add", "dockerfile-rails",
			"--optimistic", "--group", "development", "--skip-install")
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "Failed to add dockerfile-rails gem, exiting")
		}

		cmd = exec.Command(bundle, "install", "--quiet")
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "Failed to install dockerfile-rails gem, exiting")
		}
	}

	// ensure Gemfile.lock includes the x86_64-linux platform
	if out, err := exec.Command(bundle, "platform").Output(); err == nil {
		if !strings.Contains(string(out), "x86_64-linux") {
			cmd := exec.Command(bundle, "lock", "--add-platform", "x86_64-linux")
			if err := cmd.Run(); err != nil {
				return errors.Wrap(err, "Failed to add x86_64-linux platform, exiting")
			}
		}
	}

	// generate Dockerfile if it doesn't already exist
	_, err = os.Stat("Dockerfile")
	if errors.Is(err, fs.ErrNotExist) {
		flyToml := "fly.toml"
		_, err := os.Stat(flyToml)
		if os.IsNotExist(err) {
			// "touch" fly.toml
			file, err := os.Create(flyToml)
			if err != nil {
				log.Fatal(err)
			}
			file.Close()

			// inform caller of the presence of this file
			srcInfo.MergeConfig = &MergeConfigStruct{
				Name:      flyToml,
				Temporary: true,
			}
		}

		args := []string{"./bin/rails", "generate", "dockerfile",
			"--sentry", "--label=fly_launch_runtime:rails"}

		if postgres := plan.Postgres.Provider(); postgres != nil {
			args = append(args, "--postgresql", "--no-prepare")
		}

		if redis := plan.Redis.Provider(); redis != nil {
			args = append(args, "--redis")
		}

		cmd := exec.Command(ruby, args...)
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return errors.Wrap(err, "Failed to generate Dockerfile")
		}
	} else {
		if postgres := plan.Postgres.Provider(); postgres != nil && !strings.Contains(string(gemfile), "pg") {
			cmd := exec.Command(bundle, "add", "pg")
			if err := cmd.Run(); err != nil {
				return errors.Wrap(err, "Failed to install pg gem")
			}
		}

		if redis := plan.Redis.Provider(); redis != nil && !strings.Contains(string(gemfile), "redis") {
			cmd := exec.Command(bundle, "add", "redis")
			if err := cmd.Run(); err != nil {
				return errors.Wrap(err, "Failed to install redis gem")
			}
		}
	}

	// read dockerfile
	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		return errors.Wrap(err, "Dockerfile not found")
	}

	// extract port
	port := 3000
	re := regexp.MustCompile(`(?m)^EXPOSE\s+(?P<port>\d+)`)
	m := re.FindStringSubmatch(string(dockerfile))

	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "port" {
			port, err = strconv.Atoi(m[i])
			if err != nil {
				panic(err)
			}
		}
	}
	srcInfo.Port = port

	// extract volume - handle both plain string and JSON format, but only allow one path
	re = regexp.MustCompile(`(?m)^VOLUME\s+(\[\s*")?(\/[\w\/]*?(\w+))("\s*\])?\s*$`)
	m = re.FindStringSubmatch(string(dockerfile))

	if len(m) > 0 {
		srcInfo.Volumes = []Volume{
			{
				Source:      m[3], // last part of path
				Destination: m[2], // full path
			},
		}
	}

	// extract workdir
	workdir := "$"
	re = regexp.MustCompile(`(?m).*^WORKDIR\s+(?P<dir>/\S+)`)
	m = re.FindStringSubmatch(string(dockerfile))

	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "dir" {
			workdir = m[i]
		}
	}

	// add Statics if workdir is found and doesn't contain a variable reference
	if !strings.Contains(workdir, "$") {
		srcInfo.Statics = []Static{
			{
				GuestPath: workdir + "/public",
				UrlPrefix: "/",
			},
		}
	}

	// add HealthCheck (if found)
	srcInfo.HttpCheckPath = <-healthcheck_channel
	if srcInfo.HttpCheckPath != "" {
		srcInfo.HttpCheckHeaders = map[string]string{"X-Forwarded-Proto": "https"}
	}

	return nil
}
