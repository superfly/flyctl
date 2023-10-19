package scanner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command/launch/plan"
)

func configurePhoenix(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// Not phoenix, move on
	if !helpers.FileExists(filepath.Join(sourceDir, "mix.exs")) || !checksPass(sourceDir, dirContains("mix.exs", "phoenix")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family:   "Phoenix",
		Callback: PhoenixCallback,
		Concurrency: map[string]int{
			"soft_limit": 1000,
			"hard_limit": 1000,
		},
		Secrets: []Secret{
			{
				Key:  "SECRET_KEY_BASE",
				Help: "Phoenix needs a random, secret key. Use the random default we've generated, or generate your own.",
				Generate: func() (string, error) {
					return helpers.RandString(64)
				},
			},
		},
	}

	// Detect if --copy-config and --now flags are set. If so, limited set of
	// fly.toml file updates. Helpful for deploying PRs when the project is
	// already setup and we only need fly.toml config changes.
	if config.Mode == "clone" {
		s.Env = map[string]string{
			"PHX_HOST":        "APP_FQDN",
			"FLY_LAUNCH_MODE": "clone",
		}

		return s, nil
	}

	s.KillSignal = "SIGTERM"
	s.SwapSizeMB = 512
	s.Port = 8080
	s.Env = map[string]string{
		"PHX_HOST": "APP_FQDN",
		"PORT":     "8080",
	}
	s.InitCommands = []InitCommand{
		{
			Command:     "mix",
			Args:        []string{"local.rebar", "--force"},
			Description: "Preparing system for Elixir builds",
		},
		{
			Command:     "mix",
			Args:        []string{"deps.get"},
			Description: "Installing application dependencies",
		},
		{
			Command:     "mix",
			Args:        []string{"phx.gen.release", "--docker"},
			Description: "Running Docker release generator",
		},
	}

	// We found Phoenix, so check if the project compiles.
	cmd := exec.Command("mix", "do", "deps.get,", "compile")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return nil, errors.Wrap(err, "We've identified an Elixir Project but when attempting to compile it we ran into an error. Please check that your Elixir project builds successfully and try again.")
	}

	// We found Phoenix, so lets check if its a recent version.
	releaseCmd := exec.Command("mix", "run", "-e", "\"true = Code.ensure_loaded?(Mix.Tasks.Phx.Gen.Release)\"")
	releaseCmd.Stdout = os.Stdout
	releaseCmd.Stderr = os.Stderr
	err = releaseCmd.Run()
	if err == nil {
		s.DeployDocs = `
Your Phoenix app should be ready for deployment!.

If you need something else, post on our community forum at https://community.fly.io.

When you're ready to deploy, use 'fly deploy'.
`
	} else {
		s.SkipDeploy = true
		s.DeployDocs = `
We recommend upgrading to Phoenix 1.7.9 which includes a release configuration for Docker-based deployment.

If you do upgrade, you can run 'fly launch' again to get the required deployment setup.

If you don't want to upgrade, you'll need to add a few files and configuration options manually.
We've placed a Dockerfile compatible with other Phoenix 1.6 apps in this directory. See
https://hexdocs.pm/phoenix/fly.html for details, including instructions for setting up
a Postgresql database.
`
	}

	// Add migration task if we find ecto
	if checksPass(sourceDir, dirContains("mix.exs", "ecto")) {
		s.ReleaseCmd = "/app/bin/migrate"
	}

	return s, nil
}

func PhoenixCallback(appName string, _ *SourceInfo, plan *plan.LaunchPlan) error {
	envEExPath := "rel/env.sh.eex"
	envEExContents := `
# configure node for distributed erlang with IPV6 support
export ERL_AFLAGS="-proto_dist inet6_tcp"
export ECTO_IPV6="true"
export DNS_CLUSTER_QUERY="${FLY_APP_NAME}.internal"
export RELEASE_DISTRIBUTION="name"
export RELEASE_NODE="${FLY_APP_NAME}-${FLY_IMAGE_REF##*-}@${FLY_PRIVATE_IP}"
`
	_, err := os.Stat(envEExPath)
	if os.IsNotExist(err) {
		fmt.Fprintln(os.Stdout, "Generating rel/env.sh.eex for distributed Elixir support")
		contents := fmt.Sprintf("#!/bin/sh\n%s", envEExContents)

		if err := os.MkdirAll("rel", 0o755); err != nil { // skipcq: GSC-G301
			return err
		}

		if err := os.WriteFile(envEExPath, []byte(contents), 0o755); err != nil { // skipcq: GSC-G302
			return err
		}
	} else if !fileContains(envEExPath, "RELEASE_NODE") {
		fmt.Fprintln(os.Stdout, "Updating rel/env.sh.eex for distributed Elixir support")
		appendedContents := fmt.Sprintf("# appended by fly launch: configure distributed erlang with IPV6 support\n%s", envEExContents)
		f, err := os.OpenFile(envEExPath, os.O_APPEND|os.O_WRONLY, 0o755) // skipcq: GSC-G302
		if err != nil {
			return err
		}
		defer f.Close() // skipcq: GO-S2307

		if _, err := f.WriteString(appendedContents); err != nil {
			return err
		}
	}
	return nil
}
