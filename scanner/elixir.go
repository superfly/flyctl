package scanner

import (
	"path/filepath"

	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command/launch/plan"
)

func configureElixir(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
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
		Runtime: plan.RuntimeStruct{Language: "elixir"},
	}

	return s, nil
}
