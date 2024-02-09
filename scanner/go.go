package scanner

import (
	"golang.org/x/mod/modfile"
	"os"
	"path"
)

var gomod *modfile.File

func configureGo(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("go.mod")) {
		return nil, nil
	}

	modfileParsed := parseModfile()

	version := extractGoVersion(modfileParsed)
	binaryName := generateBinaryName(modfileParsed)

	files := templates("templates/go")

	s := &SourceInfo{
		Files:  files,
		Family: "Go",
		Port:   8080,
		Env: map[string]string{
			"PORT": "8080",
		},
		BuildArgs: map[string]string{
			"GO_VERSION":  version,
			"GO_BIN_NAME": binaryName,
		},
	}

	return s, nil
}

func parseModfile() bool {
	dat, err := os.ReadFile("go.mod")
	if err != nil {
		return false
	}

	f, modErr := modfile.Parse("go.mod", dat, nil)

	if modErr != nil {
		return false
	}

	gomod = f

	return true
}

func extractGoVersion(modfileParsed bool) string {
	// todo: Can we get current latest stable via web request?
	version := "1.22"
	if modfileParsed {
		// Even if it's parsed, ensure we found a version
		if len(gomod.Go.Version) > 0 {
			return gomod.Go.Version
		}
	}

	return version
}

func generateBinaryName(modfileParsed bool) string {
	binName := "main"

	longName := gomod.Module.Mod.Path

	if len(longName) > 0 {
		// Get the module name. If it's a URL or contains
		// slashes, only return the last segment
		potentialBinName := path.Base(longName)

		// Possibly the base path is just "." or "/",
		// which are not suitable names
		if len(potentialBinName) > 1 {
			return potentialBinName
		}
	}

	return binName
}
