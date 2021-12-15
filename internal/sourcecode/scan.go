package sourcecode

import (
	"bufio"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/helpers"
)

//go:embed templates/** templates/**/.dockerignore
var content embed.FS

type InitCommand struct {
	Command     string
	Args        []string
	Description string
}

type Secret struct {
	Key      string
	Help     string
	Generate bool
}
type SourceInfo struct {
	Family                string
	Version               string
	DockerfilePath        string
	Builder               string
	ReleaseCmd            string
	DockerCommand         string
	DockerEntrypoint      string
	KillSignal            string
	Buildpacks            []string
	Secrets               []Secret
	Files                 []SourceFile
	Port                  int
	Env                   map[string]string
	Statics               []Static
	Processes             map[string]string
	DeployDocs            string
	Notice                string
	SkipDeploy            bool
	Volumes               []Volume
	DockerfileAppendix    []string
	InitCommands          []InitCommand
	CreatePostgresCluster bool
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
		/* frameworks scanners are placed before generic scanners,
		   since they might mix languages or have a Dockerfile that
			 doesn't work with Fly */
		configureDockerfile,
		configureRuby,
		configureGo,
		configurePhoenix,
		configureElixir,
		configurePython,
		configureDeno,
		configureRemix,
		configureNode,
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

func SuggestAppName(sourceDir string) string {
	return filepath.Base(sourceDir)
}

type sourceScanner func(sourceDir string) (*SourceInfo, error)

func fileExists(filenames ...string) checkFn {
	return func(dir string) bool {
		for _, filename := range filenames {
			info, err := os.Stat(filepath.Join(dir, filename))
			if err != nil {
				continue
			}
			if !info.IsDir() {
				return true
			}
		}
		return false
	}
}

func fileContains(path string, pattern string) bool {
	file, err := os.Open(path)

	if err != nil {
		return false
	}

	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		re := regexp.MustCompile(pattern)
		if re.MatchString(scanner.Text()) {
			return true
		}
	}

	return false
}

func dirContains(glob string, patterns ...string) checkFn {
	return func(dir string) bool {
		for _, pattern := range patterns {
			filenames, _ := filepath.Glob(filepath.Join(dir, glob))
			for _, filename := range filenames {
				if fileContains(filename, pattern) {
					return true
				}
			}
		}
		return false
	}
}

type checkFn func(dir string) bool

func checksPass(sourceDir string, checks ...checkFn) bool {
	for _, check := range checks {
		if check(sourceDir) {
			return true
		}
	}
	return false
}

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

	// Ask to create a postgres database if we find the postgres adapter
	if checksPass(sourceDir, dirContains("mix.lock", "postgrex")) {
		s.CreatePostgresCluster = true
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
