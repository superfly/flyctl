package scanner

func configureMeteor(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists(".meteor/release")) && !checksPass(sourceDir, dirContains("package.json", "\"next\"")) {
		return nil, nil
	}

	env := map[string]string{
		"PORT": "3000",
	}

	s := &SourceInfo{
		Family:       "Meteor",
		Port:         3000,
		SkipDatabase: true,
		Env:          env,
	}

	s.Files = templates("templates/meteor")

	s.BuildArgs = map[string]string{
		"NEXT_PUBLIC_EXAMPLE": "Value goes here",
	}

	return s, nil
}
