package scanner

import (
	"fmt"
	"os"
	"strings"
)

func configureFastAPI(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("pyproject.toml", "(?i)fastapi")) {
		return nil, nil
	}
	pyProject, err := readTomlFile("pyproject.toml")
	if err != nil {
		return nil, err
	}
	deps := pyProject["tool"].(map[string]interface{})["poetry"].(map[string]interface{})["dependencies"].(map[string]interface{})
	pyVersion := deps["python"].(string)
	pyVersion = strings.TrimPrefix(pyVersion, "^")
	appName := pyProject["tool"].(map[string]interface{})["poetry"].(map[string]interface{})["name"].(string)
	appName = strings.ReplaceAll(appName, "-", "_")

	fileList, err := listSuffixedFiles(appSrc, ".py")
	if err != nil {
		return nil, err
	}

	// Scan through files to find the FastAPI app
	fastAPIFiles := make([]string, 0)
	for _, file := range fileList {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(content), "= FastAPI(") {
			fastAPIFiles = append(fastAPIFiles, file)
		}
	}

	if len(fastAPIFiles) > 1 {
		fmt.Println("Warning: Multiple files seem to contain a FastAPI app. Using the first one found.")
	}
	appFile := fastAPIFiles[0]

	vars := make(map[string]interface{})
	vars["pyVersion"] = pyVersion
	vars["appFile"] = appFile
	vars["appName"] = appName

	s := &SourceInfo{
		Files:  templatesExecute("templates/python-fastapi", vars),
		Family: "FastAPI",
		Port:   8080,
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	return s, nil
}
