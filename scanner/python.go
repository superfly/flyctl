package scanner

import "context"

func configurePython(ctx context.Context, sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("requirements.txt", "environment.yml")) {
		return nil, nil
	}

	s := &SourceInfo{
		Files:   templates("templates/python"),
		Builder: "paketobuildpacks/builder:base",
		Family:  "Python",
		Port:    8080,
		Env: map[string]string{
			"PORT": "8080",
		},
		SkipDeploy: true,
		DeployDocs: `We have generated a simple Procfile for you. Modify it to fit your needs and run "fly deploy" to deploy your application.`,
	}

	return s, nil
}
