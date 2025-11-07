package scanner

import "fmt"

func configureDeno(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(
		sourceDir,
		// default config files: https://deno.land/manual@v1.35.2/getting_started/configuration_file
		fileExists("deno.json", "deno.jsonc"),
		// deno.land and denopkg.com imports
		dirContains("*.ts", "\"https?://deno\\.land/.*\"", "\"https?://denopkg\\.com/.*\""),
	) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Deno",
		Port:   8080,
		Processes: map[string]string{
			"app": "run --allow-net ./example.ts",
		},
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	hasDockerfile := checksPass(sourceDir, fileExists("Dockerfile"))
	if hasDockerfile {
		s.DockerfilePath = "Dockerfile"
		fmt.Printf("Detected existing Dockerfile, will use it for Deno app\n")
	} else {
		s.Files = templates("templates/deno")
	}

	return s, nil
}
