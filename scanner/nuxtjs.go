package scanner

import "fmt"

func configureNuxt(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("nuxt.config.ts")) {
		return nil, nil
	}

	env := map[string]string{
		"PORT": "8080",
	}

	s := &SourceInfo{
		Family:       "NuxtJS",
		Port:         8080,
		SkipDatabase: true,
		Env:          env,
	}

	hasDockerfile := checksPass(sourceDir, fileExists("Dockerfile"))
	if hasDockerfile {
		s.DockerfilePath = "Dockerfile"
		fmt.Printf("Detected existing Dockerfile, will use it for Nuxt.js app\n")
	} else {
		s.Files = templates("templates/nuxtjs")
	}

	return s, nil
}
