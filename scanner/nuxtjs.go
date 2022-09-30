package scanner

func configureNuxt(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("nuxt.config.js")) {
		return nil, nil
	}

	env := map[string]string{
		"PORT": "8080",
	}

	s := &SourceInfo{
		Family:       "NuxtJS",
		Port:         8080,
		SkipDatabase: true,
		Env: env,
	}

	s.Files = templates("templates/nuxtjs")

	return s, nil
}
