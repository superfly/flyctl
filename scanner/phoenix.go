package scanner

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	releaseCmd := exec.Command("mix", "run", "-e", "true = Code.ensure_loaded?(Mix.Tasks.Phx.Gen.Release)")
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
a Postgres database.
`
	}

	if checksPass(sourceDir, dirContains("mix.exs", "postgrex")) {
		s.DatabaseDesired = DatabaseKindPostgres
		s.ReleaseCmd = "/app/bin/migrate"
	} else if checksPass(sourceDir, dirContains("mix.exs", "ecto_sqlite3")) {
		s.DatabaseDesired = DatabaseKindSqlite
		s.ObjectStorageDesired = true
		s.Env["DATABASE_PATH"] = "/mnt/name/name.db"
		s.Volumes = []Volume{
			{
				Source:                  "name",
				Destination:             "/mnt/name",
				InitialSize:             "1GB",
				AutoExtendSizeThreshold: 80,
				AutoExtendSizeIncrement: "1GB",
				AutoExtendSizeLimit:     "10GB",
			},
		}
	}

	if checksPass(sourceDir, dirContains("mix.exs", "redis")) {
		s.RedisDesired = true
	}

	if checksPass(sourceDir, dirContains("mix.exs", "ex_aws_s3")) {
		s.ObjectStorageDesired = true
	}

	return s, nil
}

func PhoenixCallback(appName string, srcInfo *SourceInfo, plan *plan.LaunchPlan, flags []string) error {
	envEExPath := "rel/env.sh.eex"
	envEExContents := `
# configure node for distributed erlang with IPV6 support
export ERL_AFLAGS="-proto_dist inet6_tcp"
export ECTO_IPV6="true"
export DNS_CLUSTER_QUERY="${FLY_APP_NAME}.internal"
export RELEASE_DISTRIBUTION="name"
export RELEASE_NODE="${FLY_APP_NAME}-${FLY_IMAGE_REF##*-}@${FLY_PRIVATE_IP}"

# Uncomment to send crash dumps to stderr
# This can be useful for debugging, but may log sensitive information
# export ERL_CRASH_DUMP=/dev/stderr
# export ERL_CRASH_DUMP_BYTES=4096
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

	// add Litestream if object storage is present and database is sqlite3
	if plan.ObjectStorage.Provider() != nil && srcInfo.DatabaseDesired == DatabaseKindSqlite {
		srcInfo.PostInitCallback = install_litestream
	}

	return nil
}

// Read the Dockerfile and insert the necessary commands to install Litestream
// and run the Litestream script as the entrypoint.  Primary constraint:
// do no harm.  If the Dockerfile is not in the expected format, do not modify it.
func install_litestream() error {
	// Ensure config directory exists
	if _, err := os.Stat("config"); os.IsNotExist(err) {
		return nil
	}

	// Open original Dockerfile
	file, err := os.Open("Dockerfile")
	if err != nil {
		return err
	}
	defer file.Close()

	// Create temporary output
	var lines []string

	// Variables to track state
	workdir := ""
	scanner := bufio.NewScanner(file)
	insertedLitestreamInstall := false
	foundEntrypoint := false
	insertedEntrypoint := false
	installedWget := false
	copiedLitestream := false

	// Read line by line
	for scanner.Scan() {
		line := scanner.Text()

		// Insert litestream script as entrypoint
		if strings.HasPrefix(strings.TrimSpace(line), "CMD ") && !insertedEntrypoint {
			script := workdir + "/bin/litestream.sh"

			if foundEntrypoint {
				if strings.Contains(line, "CMD [") {
					// JSON array format: CMD ["cmd"]
					line = strings.Replace(line, "CMD [", fmt.Sprintf("CMD [\"/bin/bash\", \"%s\",", script), 1)
					insertedEntrypoint = true
				} else if strings.Contains(line, "CMD \"") {
					// Shell format with quotes: CMD "cmd"
					line = strings.Replace(line, "CMD \"", fmt.Sprintf("CMD \"/bin/bash %s", script), 1)
					insertedEntrypoint = true
				}
			} else {
				lines = append(lines, "# Run litestream script as entrypoint")
				lines = append(lines, fmt.Sprintf("ENTRYPOINT [\"/bin/bash\", \"%s\"]", script))
				lines = append(lines, "")
				insertedEntrypoint = true
			}
		}

		// Add wget to install litestream
		if strings.Contains(line, "build-essential") && !installedWget {
			line = strings.Replace(line, "build-essential", "build-essential wget", 1)
			installedWget = true
		}

		// Copy litestream binary from build stage, and setup from source
		if strings.HasPrefix(strings.TrimSpace(line), "USER ") && !copiedLitestream {
			lines = append(lines, "# Copy Litestream binary from build stage")
			lines = append(lines, "COPY --from=builder /usr/bin/litestream /usr/bin/litestream")
			lines = append(lines, "COPY litestream.sh /app/bin/litestream.sh")
			lines = append(lines, "COPY config/litestream.yml /etc/litestream.yml")
			lines = append(lines, "")
			copiedLitestream = true
		}

		// Append original line
		lines = append(lines, line)

		// Install litestream
		if strings.Contains(line, "apt-get clean") && !insertedLitestreamInstall {
			lines = append(lines, "")
			lines = append(lines, "# Install litestream")
			lines = append(lines, "ARG LITESTREAM_VERSION=0.3.13")
			lines = append(lines, "RUN wget https://github.com/benbjohnson/litestream/releases/download/v${LITESTREAM_VERSION}/litestream-v${LITESTREAM_VERSION}-linux-amd64.deb \\")
			lines = append(lines, "    && dpkg -i litestream-v${LITESTREAM_VERSION}-linux-amd64.deb")

			insertedLitestreamInstall = true
		}

		// Check for existing entrypoint
		if strings.HasPrefix(strings.TrimSpace(line), "ENTRYPOINT ") {
			foundEntrypoint = true
		}

		// Track WORKDIR
		if strings.HasPrefix(strings.TrimSpace(line), "WORKDIR ") {
			workdir = strings.Split(strings.TrimSpace(line), " ")[1]
			workdir = strings.Trim(workdir, "\"")
			workdir = strings.TrimRight(workdir, "/")
		}
	}

	// Check for errors
	if err := scanner.Err(); err != nil {
		return err
	}

	// If we didn't complete the insertion, return without writing to file
	if !insertedLitestreamInstall || !insertedEntrypoint || !copiedLitestream {
		fmt.Println("Failed to insert Litestream installation commands. Skipping Litestream installation.")
		return nil
	} else {
		fmt.Fprintln(os.Stdout, "Updating Dockerfile to install Litestream")
	}

	// Write dockerfile back to file
	dockerfile, err := os.Create("Dockerfile")
	if err != nil {
		return err
	}
	defer dockerfile.Close()

	for _, line := range lines {
		fmt.Fprintln(dockerfile, line)
	}

	// Create litestream.sh
	script, err := os.Create("litestream.sh")
	if err != nil {
		return bufio.ErrBadReadCount
	}
	defer script.Close()

	_, err = fmt.Fprint(script, strings.TrimSpace(`
#!/usr/bin/env bash
set -e

# If db doesn't exist, try restoring from object storage
if [ ! -f "$DATABASE_PATH" ] && [ -n "$BUCKET_NAME" ]; then
	litestream restore -if-replica-exists "$DATABASE_PATH"
fi

# Migrate database
/app/bin/migrate

# Launch application
if [ -n "$BUCKET_NAME" ]; then
	litestream replicate -exec "${*}"
else
	exec "${@}"
fi
	`))

	if err != nil {
		return err
	}

	// Create litestream.yml
	config, err := os.Create("config/litestream.yml")
	if err != nil {
		return err
	}

	defer config.Close()

	_, err = fmt.Fprint(config, strings.TrimSpace(strings.ReplaceAll(`
# This is the configuration file for litestream.
#
# For more details, see: https://litestream.io/reference/config/
#
dbs:
- path: $DATABASE_PATH
  replicas:
  - type: s3
	endpoint: $AWS_ENDPOINT_URL_S3
	bucket: $BUCKET_NAME
	path: litestream${DATABASE_PATH}
	access-key-id: $AWS_ACCESS_KEY_ID
	secret-access-key: $AWS_SECRET_ACCESS_KEY
	region: $AWS_REGION
`, "\t", "    ")))

	return err
}
