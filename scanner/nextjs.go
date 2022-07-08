package scanner

func configureNextJs(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("next.config.js")) && !checksPass(sourceDir, dirContains("package.json", "\"next\"")) {
		return nil, nil
	}

	env := map[string]string{
		"PORT": "8080",
	}

	s := &SourceInfo{
		Family:       "NextJS",
		Port:         8080,
		SkipDatabase: true,
	}

	s.Files = templates("templates/nextjs")

	s.BuildArgs = map[string]string{
		"NEXT_PUBLIC_EXAMPLE": "Value goes here",
	}

	s.Env = env
	return s, nil
}
