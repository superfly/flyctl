package scanner

func configureMaven(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("pom.xml")) {
		return nil, nil
	}

	env := map[string]string{
		"PORT": "8080",
	}

	s := &SourceInfo{
		Family:       "Maven",
		Port:         8080,
		SkipDatabase: true,
		Env:          env,
	}

	s.Files = templates("templates/maven")

	return s, nil
}
