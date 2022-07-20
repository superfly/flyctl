package scanner

func configureNode(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("package.json")) {
		return nil, nil
	}

	s := &SourceInfo{
		Builder: "heroku/buildpacks:20",
		Family:  "NodeJS",
		Port:    8080,
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}
