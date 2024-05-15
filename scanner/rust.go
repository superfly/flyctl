package scanner

import  (
	"fmt"
	"os"
	"github.com/pelletier/go-toml/v2"
)

func readCargoFile() (map[string]interface{}, error) {
	doc, err := os.ReadFile("Cargo.toml")
	if err != nil {
		return nil, fmt.Errorf("Error reading Cargo file:", err)
	}
	cargoData := make(map[string]interface{})
	readErr := toml.Unmarshal(doc, &cargoData)
	if readErr != nil {
		return nil, fmt.Errorf("Error parsing Cargo file:", readErr)
	}
	return cargoData, nil
}

func configureRust(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Cargo.toml", "Cargo.lock")) {
		return nil, nil
	}

	cargoData, err := readCargoFile()
	if err != nil {
		return nil, err
	}

	deps := cargoData["dependencies"].(map[string]interface{})
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

	vars := make(map[string]interface{})
	vars["appName"] = cargoData["package"].(map[string]interface{})["name"].(string)

	s := &SourceInfo{
		Files:      	templatesExecute("templates/rust", vars),
		Family:     	family,
		Port:       	8080,
		Env:			env,
		SkipDatabase: 	true,
	}
	return s, nil
}