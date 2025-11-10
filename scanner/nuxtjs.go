package scanner

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

	if hasDockerfile, dockerfilePath := checkExistingDockerfile(sourceDir, "Nuxt.js"); hasDockerfile {
		s.DockerfilePath = dockerfilePath
	} else {
		s.Files = templates("templates/nuxtjs")
	}

	return s, nil
}
