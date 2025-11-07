package scanner

import "fmt"

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

	hasDockerfile := checksPass(sourceDir, fileExists("Dockerfile"))
	if hasDockerfile {
		s.DockerfilePath = "Dockerfile"
		fmt.Printf("Detected existing Dockerfile, will use it for Next.js app\n")
	} else {
		s.Files = templates("templates/nextjs")
	}

	s.BuildArgs = map[string]string{
		"NEXT_PUBLIC_EXAMPLE": "Value goes here",
	}

	return s, nil
}
