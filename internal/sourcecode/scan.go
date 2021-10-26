package sourcecode

import (
	"bufio"
	"embed"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/helpers"
)

//go:embed templates/** templates/**/.dockerignore
var content embed.FS

type SourceInfo struct {
	Family         string
	DockerfilePath string
	Builder        string
	Buildpacks     []string
	Secrets        map[string]string
	Files          []SourceFile
	Port           int
	Env            map[string]string
	Statics        []Static
	Processes      map[string]string
	DeployDocs     string
	SkipDeploy     bool
}

type SourceFile struct {
	Path     string
	Contents []byte
}
type Static struct {
	GuestPath string `toml:"guest_path" json:"guest_path"`
	UrlPrefix string `toml:"url_prefix" json:"url_prefix"`
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
		configureElixir,
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
		if strings.Contains(scanner.Text(), pattern) {
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

func configureElixir(sourceDir string) (*SourceInfo, error) {
	if !helpers.FileExists(filepath.Join(sourceDir, "mix.exs")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder:    "heroku/buildpacks:20",
		Buildpacks: []string{"https://cnb-shim.herokuapp.com/v1/hashnuke/elixir"},
		Family:     "Elixir",
		Secrets: map[string]string{
			"SECRET_KEY_BASE": "The input secret for the application key generator. Use something long and random.",
		},
		Port: 8080,
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

	s := &SourceInfo{
		Family: "Remix",
		Files:  templates("templates/remix"),
		Port:   8080,
		Secrets: map[string]string{
			"REMIX_TOKEN": "Your Remix authentication token.",
		},
		Env: map[string]string{
			"PORT": "8080",
		},
		SkipDeploy: true,
		DeployDocs: `To deploy this app, run 'npm run deploy'`,
	}

	return s, nil
}

// templates recursively returns files from the templates directory within the named directory
// will panic on errors since these files are embedded and should work
func templates(name string) (files []SourceFile) {
	err := fs.WalkDir(content, name, func(path string, d fs.DirEntry, err error) error {
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
