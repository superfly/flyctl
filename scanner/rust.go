package scanner

import (
	"fmt"

	"github.com/superfly/flyctl/internal/command/launch/plan"
)

func configureRust(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Cargo.toml", "Cargo.lock")) {
		return nil, nil
	}

	cargoData, err := readTomlFile("Cargo.toml")
	if err != nil {
		return nil, err
	}

	// Cargo.toml may not contain a "dependencies" section, so we don't return an error if it's missing.
	deps, _ := cargoData["dependencies"].(map[string]interface{})
	family := "Rust"
	env := map[string]string{
		"PORT": "8080",
	}

	if _, ok := deps["rocket"]; ok {
		family = "Rocket"
		env["ROCKET_PORT"] = "8080"
		env["ROCKET_ADDRESS"] = "0.0.0.0"
	} else if _, ok := deps["actix-web"]; ok {
		family = "Actix Web"
	} else if _, ok := deps["warp"]; ok {
		family = "Warp"
	} else if _, ok := deps["axum"]; ok {
		family = "Axum"
	} else if _, ok := deps["poem"]; ok {
		family = "Poem"
	}

	pkg, ok := cargoData["package"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("file Cargo.toml does not contain a valid package section")
	}

	vars := make(map[string]interface{})
	vars["appName"], ok = pkg["name"].(string)
	if !ok {
		return nil, fmt.Errorf("file Cargo.toml does not contain a valid package name")
	}

	s := &SourceInfo{
		Files:        templatesExecute("templates/rust", vars),
		Family:       family,
		Port:         8080,
		Env:          env,
		SkipDatabase: true,
		Runtime:      plan.RuntimeStruct{Language: "rust"},
	}

	// TODO: check Cargo.toml's `package.rust-version` and rust-toolchain.toml's `toolchain.channel`
	return s, nil
}
