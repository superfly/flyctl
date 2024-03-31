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

	// Generate a simple Dockerfile
	s := &SourceInfo{
		Files:      templatesExecute("templates/flask", vars),
		Family:     "Flask",
		Port:       8080,
		SkipDeploy: true,
		DeployDocs: `We have generated a simple Dockerfile for you. Modify it to fit your needs and run "fly deploy" to deploy your application.`,
	}

	return s, nil
}
