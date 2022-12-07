package scanner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/flag"
)

func configurePhoenix(sourceDir string, ctx context.Context) (*SourceInfo, error) {
	// Not phoenix, move on
	if !helpers.FileExists(filepath.Join(sourceDir, "mix.exs")) || !checksPass(sourceDir, dirContains("mix.exs", "phoenix")) {
		return nil, nil
	}

	// Detect if --copy-config and --now flags are set. If so, limited set of
	// fly.toml file updates. Helpful for deploying PRs when the project is
	// already setup and we only need fly.toml config changes.
	if flag.GetBool(ctx, "copy-config") && flag.GetBool(ctx, "now") {
		s := &SourceInfo{
			Family: "Phoenix",
			Secrets: []Secret{
				{
					Key:  "SECRET_KEY_BASE",
					Help: "Phoenix needs a random, secret key. Use the random default we've generated, or generate your own.",
					Generate: func() (string, error) {
						return helpers.RandString(64)
					},
				},
			},
			Env: map[string]string{
				"PHX_HOST":        "APP_FQDN",
				"FLY_LAUNCH_MODE": "copy-config-now",
			},
		}

		return s, nil
	}

	s := &SourceInfo{
		Family: "Phoenix",
		Secrets: []Secret{
			{
				Key:  "SECRET_KEY_BASE",
				Help: "Phoenix needs a random, secret key. Use the random default we've generated, or generate your own.",
				Generate: func() (string, error) {
					return helpers.RandString(64)
				},
			},
		},
		KillSignal: "SIGTERM",
		Port:       8080,
		Env: map[string]string{
			"PORT":     "8080",
			"PHX_HOST": "APP_FQDN",
		},
		DockerfileAppendix: []string{
			"ENV ECTO_IPV6 true",
			"ENV ERL_AFLAGS \"-proto_dist inet6_tcp\"",
		},
		InitCommands: []InitCommand{
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
		},
	}

	// We found Phoenix, so check if the Docker generator is present
	cmd := exec.Command("mix", "do", "deps.get,", "compile,", "run", "-e", "\"true = Code.ensure_loaded?(Mix.Tasks.Phx.Gen.Release)\"")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		s.DeployDocs = `
Your Phoenix app should be ready for deployment!.

If you need something else, post on our community forum at https://community.fly.io.

When you're ready to deploy, use 'fly deploy'.
`
	} else {
		s.SkipDeploy = true
		s.DeployDocs = `
We recommend upgrading to Phoenix 1.6.3 which includes a release configuration for Docker-based deployment.

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
