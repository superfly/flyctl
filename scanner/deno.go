package scanner

func configureDeno(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("*.ts", "denopkg")) {
		return nil, nil
	}

	s := &SourceInfo{
		Files:  templates("templates/deno"),
		Family: "Deno",
		Port:   8080,
		Processes: map[string]string{
			"app": "run --allow-net ./example.ts",
		},
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}
