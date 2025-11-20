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

	s.Files = templates("templates/nuxtjs")

	// detect node.js version properly...
	if nodeS, err := configureNode(sourceDir, config); err == nil && nodeS != nil {
		s.Runtime = nodeS.Runtime
	}

	return s, nil
}
