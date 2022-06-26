package sourcecode

import (
	"embed"
	"fmt"
	"io/fs"
	"io/ioutil"
	"path/filepath"
	"os/exec"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/helpers"
)

//go:embed templates templates/*/.dockerignore templates/*/*/.dockerignore templates/**/.fly
var content embed.FS

type InitCommand struct {
	Command     string
	Args        []string
	Description string
	Condition   bool
}

type Secret struct {
	Key      string
	Help     string
	Value    string
	Generate bool
}
type SourceInfo struct {
	Family                       string
	Version                      string
	DockerfilePath               string
	BuildArgs                    map[string]string
	Builder                      string
	ReleaseCmd                   string
	DockerCommand                string
	DockerEntrypoint             string
	KillSignal                   string
	Buildpacks                   []string
	Secrets                      []Secret
	Files                        []SourceFile
	Port                         int
	Env                          map[string]string
	Statics                      []Static
	Processes                    map[string]string
	DeployDocs                   string
	Notice                       string
	SkipDeploy                   bool
	SkipDatabase                 bool
	Volumes                      []Volume
	DockerfileAppendix           []string
	InitCommands                 []InitCommand
	PostgresInitCommands         []InitCommand
	PostgresInitCommandCondition bool
}

type SourceFile struct {
	Path     string
	Contents []byte
}
type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix"`
}
type Volume struct {
	Source      string `toml:"source" json:"source"`
	Destination string `toml:"destination" json:"destination"`
}

func Scan(sourceDir string) (*SourceInfo, error) {
	scanners := []sourceScanner{
		configureDjango,
		configureLaravel,
		configurePhoenix,
		configureRails,
		configureRedwood,
		/* frameworks scanners are placed before generic scanners,
		   since they might mix languages or have a Dockerfile that
			 doesn't work with Fly */
		configureDockerfile,
		configureLucky,
		configureRuby,
		configureGo,
		configureElixir,
		configurePython,
		configureDeno,
		configureRemix,
		configureNuxt,
		configureNode,
		configureStatic,
	}

	for _, scanner := range scanners {
		si, err := scanner(sourceDir)
		if err != nil {
			return nil, err
		}
		if si != nil {
			return si, nil
		}
	}

	return nil, nil
}

type sourceScanner func(sourceDir string) (*SourceInfo, error)

func configureDockerfile(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Dockerfile")) {
		return nil, nil
	}

	s := &SourceInfo{
		DockerfilePath: filepath.Join(sourceDir, "Dockerfile"),
		Family:         "Dockerfile",
	}

	return s, nil
}

func configureLucky(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("shard.yml", "lucky")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family:     "Lucky",
		Files:      templates("templates/lucky"),
		Port:       8080,
		ReleaseCmd: "lucky db.migrate",
		Env: map[string]string{
			"PORT":       "8080",
			"LUCKY_ENV":  "production",
			"APP_DOMAIN": "APP_FQDN",
		},
		Secrets: []Secret{
			{
				Key:      "SECRET_KEY_BASE",
				Help:     "Lucky needs a random, secret key. Use the random default we've generated, or generate your own.",
				Generate: true,
			},
			{
				Key:   "SEND_GRID_KEY",
				Help:  "Lucky needs a SendGrid API key. For now, we're setting this to unused. You can generate one at https://docs.sendgrid.com/for-developers/sending-email/api-getting-started",
				Value: "unused",
			},
		},
		Statics: []Static{
			{
				GuestPath: "/app/public",
				UrlPrefix: "/",
			},
		},
	}

	return s, nil
}

func configureRails(sourceDir string) (*SourceInfo, error) {

	if !checksPass(sourceDir, dirContains("Gemfile", "rails")) {
		return nil, nil
	}

	s := &SourceInfo{
		Files:  templates("templates/rails/standard"),
		Family: "Rails",
		Statics: []Static{
			{
				GuestPath: "/app/public",
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
		ReleaseCmd: "bundle exec rails db:migrate",
		Env: map[string]string{
			"SERVER_COMMAND": "bundle exec puma -C config/puma.rb",
			"PORT":           "8080",
		},
	}

	var rubyVersion string
	var bundlerVersion string
	var nodeVersion string = "14"

	rubyVersion, err := extractRubyVersion("Gemfile", ".ruby_version")

	if err != nil || rubyVersion == "" {
		rubyVersion = "3.1.1"
	}

	bundlerVersion, err = extractBundlerVersion("Gemfile.lock")

	if err != nil || bundlerVersion == "" {
		bundlerVersion = "2.3.9"
	}

	s.BuildArgs = map[string]string{
		"RUBY_VERSION":    rubyVersion,
		"BUNDLER_VERSION": bundlerVersion,
		"NODE_VERSION":    nodeVersion,
	}

	// master.key comes with Rails apps from v6 onwards, but may not be present
	// if the app does not use Rails encrypted credentials
	masterKey, err := ioutil.ReadFile("config/master.key")

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
	s.DeployDocs = fmt.Sprintf(`
Your Rails app is prepared for deployment. Production will be setup with these versions of core runtime packages:

Ruby %s
Bundler %s
NodeJS %s

You can configure these in the [build] section in the generated fly.toml.

Ruby versions available are: 3.1.1, 3.0.3, 2.7.5, and 2.6.9. Learn more about the chosen Ruby stack, Fullstaq Ruby, here: https://github.com/evilmartians/fullstaq-ruby-docker.
We recommend using the highest patch level for better security and performance.

For the other packages, specify any version you need.

If you need custom packages installed, or have problems with your deployment build, you may need to edit the Dockerfile
for app-specific changes. If you need help, please post on https://community.fly.io.

Now: run 'fly deploy --remote-only' to deploy your Rails app.
`, rubyVersion, bundlerVersion, nodeVersion)

	return s, nil

}

func configureRuby(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Gemfile", "config.ru")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder: "heroku/buildpacks:20",
		Family:  "Ruby",
		Port:    8080,
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}

func configureGo(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("go.mod", "Gopkg.lock")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder:    "paketobuildpacks/builder:base",
		Buildpacks: []string{"gcr.io/paketo-buildpacks/go"},
		Family:     "Go",
		Port:       8080,
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}

func configurePython(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("requirements.txt", "environment.yml")) {
		return nil, nil
	}

	s := &SourceInfo{
		Files:   templates("templates/python"),
		Builder: "paketobuildpacks/builder:base",
		Family:  "Python",
		Port:    8080,
		Env: map[string]string{
			"PORT": "8080",
		},
		SkipDeploy: true,
		DeployDocs: `We have generated a simple Procfile for you. Modify it to fit your needs and run "fly deploy" to deploy your application.`,
	}

	return s, nil
}

func configureNode(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("package.json")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder: "heroku/buildpacks:20",
		Family:  "NodeJS",
		Port:    8080,
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}

func configureDeno(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("*.ts", "denopkg")) {
		return nil, nil
	}

	s := &SourceInfo{
		Files:  templates("templates/deno"),
		Family: "Deno",
		Port:   8080,
		Processes: map[string]string{
			"app": "run --allow-net ./example.ts",
		},
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}

func configureStatic(sourceDir string) (*SourceInfo, error) {
	// No index.html detected, move on
	if !helpers.FileExists(filepath.Join(sourceDir, "index.html")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Static",
		Port:   8080,
		Files:  templates("templates/static"),
	}

	return s, nil

}

func configurePhoenix(sourceDir string) (*SourceInfo, error) {
	// Not phoenix, move on
	if !helpers.FileExists(filepath.Join(sourceDir, "mix.exs")) || !checksPass(sourceDir, dirContains("mix.exs", "phoenix")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Phoenix",
		Secrets: []Secret{
			{
				Key:      "SECRET_KEY_BASE",
				Help:     "Phoenix needs a random, secret key. Use the random default we've generated, or generate your own.",
				Generate: true,
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
  cmd := exec.Command("mix", "help", "phx.gen.release")
  err := cmd.Run()
	if err == nil {
		s.DeployDocs = `
Your Phoenix app should be ready for deployment!.

If you need something else, post on our community forum at https://community.fly.io.

When you're ready to deploy, use 'fly deploy --remote-only'.
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

func configureElixir(sourceDir string) (*SourceInfo, error) {
	if !helpers.FileExists(filepath.Join(sourceDir, "mix.exs")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder:    "heroku/buildpacks:20",
		Buildpacks: []string{"https://cnb-shim.herokuapp.com/v1/hashnuke/elixir"},
		Family:     "Elixir",
		Port:       8080,
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}

func configureRedwood(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("redwood.toml")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family:     "RedwoodJS",
		Files:      templates("templates/redwood"),
		Port:       8910,
		ReleaseCmd: ".fly/release.sh",
	}

	s.Env = map[string]string{
		"PORT": "8910",
		// Telemetry gravely incrases memory usage, and isn't required
		"REDWOOD_DISABLE_TELEMETRY": "1",
	}

	if checksPass(sourceDir+"/api/db", dirContains("*.prisma", "sqlite")) {
		s.Env["MIGRATE_ON_BOOT"] = "true"
		s.Env["DATABASE_URL"] = "file://data/sqlite.db"
		s.Volumes = []Volume{
			{
				Source:      "data",
				Destination: "/data",
			},
		}
		s.Notice = "\nThis deployment will run an SQLite on a single dedicated volume. The app can't scale beyond a single instance. Look into 'fly postgres' for a more robust production database that supports scaling up. \n"
	}

	return s, nil
}

func configureRemix(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("remix.config.js")) {
		return nil, nil
	}

	env := map[string]string{
		"PORT": "8080",
	}

	s := &SourceInfo{
		Family: "Remix",
		Port:   8080,
	}

	if checksPass(sourceDir+"/prisma", dirContains("*.prisma", "sqlite")) {
		env["DATABASE_URL"] = "file:/data/sqlite.db"
		s.Files = templates("templates/remix_prisma")
		s.DockerCommand = "start_with_migrations.sh"
		s.DockerEntrypoint = "sh"
		s.Volumes = []Volume{
			{
				Source:      "data",
				Destination: "/data",
			},
		}
		s.Notice = "\nThis launch configuration uses SQLite on a single, dedicated volume. It will not scale beyond a single VM. Look into 'fly postgres' for a more robust production database. \n"
	} else {
		s.Files = templates("templates/remix")
	}

	s.Env = env
	return s, nil
}

func configureNuxt(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("nuxt.config.js")) {
		return nil, nil
	}

	env := map[string]string{
		"PORT": "8080",
	}

	s := &SourceInfo{
		Family:       "NuxtJS",
		Port:         8080,
		SkipDatabase: true,
	}

	s.Files = templates("templates/nuxtjs")

	s.Env = env
	return s, nil
}

// setup django with a postgres database
func configureDjango(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("requirements.txt", "manage.py")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Django",
		Port:   8080,
		Files:  templates("templates/django"),
		Env: map[string]string{
			"PORT": "8080",
		},
		Secrets: []Secret{
			{
				Key:      "SECRET_KEY",
				Help:     "Django needs a random, secret key. Use the random default we've generated, or generate your own.",
				Generate: true,
			},
		},
		Statics: []Static{
			{
				GuestPath: "/app/public",
				UrlPrefix: "/static/",
			},
		},
		SkipDeploy: true,
	}

	// check if requirements.txt has a postgres dependency
	if checksPass(sourceDir, dirContains("requirements.txt", "psycopg2")) {
		s.InitCommands = []InitCommand{
			{
				// python makemigrations
				Command:     "python",
				Args:        []string{"manage.py", "makemigrations"},
				Description: "Creating database migrations",
			},
		}
		s.ReleaseCmd = "python manage.py migrate"

		if !checksPass(sourceDir, dirContains("requirements.txt", "database_url")) {
			s.DeployDocs = `
Your Django app is almost ready to deploy!

We recommend using the database_url(pip install dj-database-url) to parse the DATABASE_URL from os.environ['DATABASE_URL']

For detailed documentation, see https://fly.dev/docs/django/
		`
		} else {

			s.DeployDocs = `
Your Django app is ready to deploy!

For detailed documentation, see https://fly.dev/docs/django/
		`
		}
	}

	return s, nil
}

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
			"APP_ENV":     "production",
			"LOG_CHANNEL": "stderr",
			"LOG_LEVEL":   "info",
		},
		Family: "Laravel",
		Files:  files,
		Port:   8080,
		Secrets: []Secret{
			{
				Key:  "APP_KEY",
				Help: "Laravel needs a unique application key. Use 'php artisan key:generate --show' to generate this value.",
				// TODO: Can we generate this for users?
			},
		},
		SkipDatabase: true,
	}

	return s, nil
}

// templates recursively returns files from the templates directory within the named directory
// will panic on errors since these files are embedded and should work
func templates(name string) (files []SourceFile) {
	err := fs.WalkDir(content, name, func(path string, d fs.DirEntry, e error) error {
		if d.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(name, path)

		if err != nil {
			return errors.Wrap(err, "error removing template prefix")
		}

		data, err := fs.ReadFile(content, path)

		if err != nil {
			return err
		}

		if err != nil {
			return err
		}

		f := SourceFile{
			Path:     relPath,
			Contents: data,
		}

		files = append(files, f)
		return nil
	})

	if err != nil {
		panic(err)
	}

	return
}
