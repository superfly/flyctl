package scanner

import "fmt"

func configureFlask(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	// require "Flask" to be in requirements.txt
	if !checksPass(sourceDir, dirContains("requirements.txt", "Flask")) {
		return nil, nil
	}

	// require "app.py" or "wsgi.py" to be in the root directory
	if !checksPass(sourceDir, fileExists("app.py", "wsgi.py")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family:     "Flask",
		Port:       8080,
		SkipDeploy: true,
		DeployDocs: `We have generated a simple Dockerfile for you. Modify it to fit your needs and run "fly deploy" to deploy your application.`,
	}

	hasDockerfile := checksPass(sourceDir, fileExists("Dockerfile"))
	if hasDockerfile {
		s.DockerfilePath = "Dockerfile"
		fmt.Printf("Detected existing Dockerfile, will use it for Flask app\n")
	} else {
		vars := make(map[string]interface{})

		// Extract Python version
		// TODO: support pinned versions
		pythonFullVersion, _, err := extractPythonVersion()
		if err != nil {
			return nil, err
		} else if pythonFullVersion == "" {
			return nil, fmt.Errorf("could not find Python version")
		}
		vars["pythonVersion"] = pythonFullVersion

		s.Files = templatesExecute("templates/flask", vars)
	}

	return s, nil
}
