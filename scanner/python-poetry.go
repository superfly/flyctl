package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

func findEntrypoint(dep string) *os.File {
	var entrypoint *os.File = nil
	filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if filepath.Ext(path) == ".py" && !strings.Contains(path, ".venv") {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close() // skipcq: GO-S2307

			scanner := bufio.NewScanner(file)
			for scanner.Scan() {
				line := scanner.Text()
				if strings.Contains(line, "import") && strings.Contains(line, dep) {
					entrypoint = file
				}
			}

			if err := scanner.Err(); err != nil {
				return err
			}
		}
		return nil
	})
	return entrypoint
}

func configurePoetry(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("pyproject.toml", "poetry.lock")) {
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

	vars := make(map[string]interface{})
	vars["pyVersion"] = pyVersion
	vars["appName"] = appName

	if _, ok := deps["fastapi"]; ok {
		return &SourceInfo{
			Files:  templatesExecute("templates/python-fastapi", vars),
			Family: "FastAPI",
			Port:   8000,
		}, nil
	} else if _, ok := deps["flask"]; ok {
		return &SourceInfo{
			Files:  templatesExecute("templates/python-flask-poetry", vars),
			Family: "Flask",
			Port:   8080,
		}, nil
	} else if _, ok := deps["streamlit"]; ok {
		entrypoint := findEntrypoint("streamlit")
		if entrypoint == nil {
			return nil, nil
		} else {
			vars["entrypoint"] = entrypoint.Name()
		}
		return &SourceInfo{
			Files:  templatesExecute("templates/python-streamlit", vars),
			Family: "Streamlit",
			Port:   8501,
		}, nil
	} else {
		return nil, nil
	}

}
