package scanner

func configureNextJs(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
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
		Env:          env,
	}

	s.Files = templates("templates/nextjs")

	s.BuildArgs = map[string]string{
		"NEXT_PUBLIC_EXAMPLE": "Value goes here",
	}

	// detect node.js version properly...
	if nodeS, err := configureNode(sourceDir, config); err == nil && nodeS != nil {
		s.Runtime = nodeS.Runtime
	}

	return s, nil
}
