package sourcecode

import (
	"embed"
	"io/fs"
	"io/ioutil"
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/helpers"
)

//go:embed templates templates/*/.dockerignore
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
		configureRedwood,
		configureDjango,
		/* frameworks scanners are placed before generic scanners,
		   since they might mix languages or have a Dockerfile that
			 doesn't work with Fly */
		configureDockerfile,
		configureRails,
		configureRuby,
		configureGo,
		configurePhoenix,
		configureElixir,
		configurePython,
		configureDeno,
		configureRemix,
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

func configureRails(sourceDir string) (*SourceInfo, error) {

	if !checksPass(sourceDir, dirContains("Gemfile", "rails")) {
		return nil, nil
	}

	s := &SourceInfo{
		Files:   templates("templates/rails"),
		Builder: "heroku/buildpacks:20",
		Family:  "Rails",
		Port:    8080,
		Env: map[string]string{
			"PORT": "8080",
		},
		InitCommands: []InitCommand{
			{
				Command:     "bundle",
				Args:        []string{"lock", "--add-platform", "x86_64-linux"},
				Description: "Preparing Gemfile.lock for x86_64 deployment",
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

	s.ReleaseCmd = "bundle exec rails db:migrate"

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

	// We found Phoenix 1.6.3 or higher, so try running the Docker generator
	if checksPass(sourceDir, dirContains("mix.exs", "phoenix.*"+regexp.QuoteMeta("1.6"))) {
		s.DeployDocs = `
Your Phoenix app should be ready for deployment!.

If you need something else, post on our community forum at https://community.fly.io.

When you're ready to deploy, use 'fly deploy --remote-only'.
`
	}
	// We found Phoenix 1.6.0 - 1.6.2
	if checksPass(sourceDir, dirContains("mix.exs", "phoenix.*"+regexp.QuoteMeta("1.6.")+"[0-2]")) {
		s.SkipDeploy = true
		s.DeployDocs = `
We recommend upgrading to Phoenix 1.6.3 which includes a release configuration for Docker-based deployment.

If you do upgrade, you can run 'fly launch' again to get the required deployment setup.

If you don't want to uprade, you'll need to add a few files and configuration options manually.
W've placed Dockerfile compatible with other Phoenix 1.6 apps in this directory. See
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
		Family: "RedwoodJS",
		Files:  templates("templates/redwood"),
		Port:   8911,
		Env: map[string]string{
			"PORT": "8911",
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
