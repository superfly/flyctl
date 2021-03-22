package sourcecode

import (
	"os"
	"path/filepath"

	"github.com/superfly/flyctl/helpers"
)

type SourceInfo struct {
	Family         string
	DockerfilePath string
	Builder        string
	Buildpacks     []string
}

func Scan(sourceDir string) (*SourceInfo, error) {
	scanners := []sourceScanner{
		configureDockerfile,
		configureRuby,
		configureGo,
		configureElixir,
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
	if !checksPass(sourceDir, fileExists("Gemfile")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder: "paketobuildpacks/builder:base",
		Buildpacks: []string{
			"gcr.io/paketo-buildpacks/ruby",
		},
		Family: "Ruby",
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
	}

	return s, nil
}

func configureNode(sourceDir string) (*SourceInfo, error) {
	if !helpers.FileExists(filepath.Join(sourceDir, "package.json")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder:    "paketobuildpacks/builder:base",
		Buildpacks: []string{"gcr.io/paketo-buildpacks/nodejs"},
		Family:     "NodeJS",
	}

	return s, nil
}

func configureElixir(sourceDir string) (*SourceInfo, error) {
	if !helpers.FileExists(filepath.Join(sourceDir, "mix.exs")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder:    "heroku/buildpacks:18",
		Buildpacks: []string{"https://cnb-shim.herokuapp.com/v1/hashnuke/elixir"},
		Family:     "Elixir",
	}

	return s, nil
}
