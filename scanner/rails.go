package scanner

import (
	"log"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func configureRails(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("Gemfile", "rails")) {
		return nil, nil
	}

	// install dockerfile-rails gem, if not already included
	gemfile, err := os.ReadFile("Gemfile")
	if err != nil {
		panic(err)
	} else if !strings.Contains(string(gemfile), "dockerfile-rails") {
		cmd := exec.Command("bundle", "add", "dockerfile-rails",
			"--version", ">= 0.5.0", "--group", "development")
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Fatal("Failed to add dockerfile-rails gem, exiting")
		}
	}

	// generate Dockerfile if it doesn't already exist
	_, err = os.Stat("Dockerfile")
	if os.IsNotExist(err) {
		cmd := exec.Command("ruby", "./bin/rails", "generate", "dockerfile")
		cmd.Stdin = nil
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Fatal("Failed to generate Dockefile, exiting")
		}
	}

	// read dockerfile
	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		log.Fatal("Dockerfile not found, exiting")
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

	// extract workdir
	workdir := "/rails"
	re = regexp.MustCompile(`(?m).*^WORKDIR\s+(?P<dir>/\S+)`)
	m = re.FindStringSubmatch(string(dockerfile))

	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "dir" {
			workdir = m[i]
		}
	}

	s := &SourceInfo{
		Family: "Rails",
		Port:   port,
		Statics: []Static{
			{
				GuestPath: workdir + "/public",
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
