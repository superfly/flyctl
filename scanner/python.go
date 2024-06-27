package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/superfly/flyctl/terminal"
	"golang.org/x/exp/slices"
)

type PyApp string

const (
	FastAPI   PyApp = "fastapi"
	Flask     PyApp = "flask"
	Streamlit PyApp = "streamlit"
)

var supportedApps = []PyApp{FastAPI, Flask, Streamlit}

type PyProjectCfg struct {
	pyVersion     string
	appName       string
	supportedApps []PyApp
}

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

func parsePyDep(dep string) string {
	// remove all version constraints from a python dependency
	// e.g. "fastapi>=0.1.0" -> "fastapi"
	// e.g. "flask" -> "flask"
	// e.g. "pytest < 5.0.0" -> "pytest"
	// e.g. "numpy~=1.19.2" -> "numpy"
	// e.g. "django>2.1; os_name != 'nt'" -> "django"
	dep = strings.ToLower(dep)
	dep = strings.Split(dep, ";")[0]
	dep = strings.Split(dep, " ")[0]
	dep = strings.Split(dep, "==")[0]
	dep = strings.Split(dep, ">")[0]
	dep = strings.Split(dep, "<")[0]
	dep = strings.Split(dep, "~=")[0]
	return dep
}

func fromPoetry(pyProject map[string]interface{}) (PyProjectCfg, error) {
	// Parse pyproject.toml managed with poetry
	deps := pyProject["tool"].(map[string]interface{})["poetry"].(map[string]interface{})["dependencies"].(map[string]interface{})
	var apps []PyApp
	for dep := range deps {
		if slices.Contains(supportedApps, PyApp(dep)) {
			apps = append(apps, PyApp(dep))
		}
	}
	pyVersion := deps["python"].(string)
	pyVersion = strings.TrimPrefix(pyVersion, "^")
	appName := pyProject["tool"].(map[string]interface{})["poetry"].(map[string]interface{})["name"].(string)

	return PyProjectCfg{pyVersion, appName, apps}, nil
}

func fromPyProject(pyProject map[string]interface{}) (PyProjectCfg, error) {
	// Parse pyproject.toml from pep 621 spec
	project := pyProject["project"].(map[string]interface{})
	deps := project["dependencies"].([]interface{})
	var depList []PyApp
	for _, dep := range deps {
		dep := dep.(string)
		dep = parsePyDep(dep)
		if slices.Contains(supportedApps, PyApp(dep)) && !slices.Contains(depList, PyApp(dep)) {
			depList = append(depList, PyApp(dep))
		}
	}
	pyVersion := project["requires-python"].(string)
	pyVersion = strings.TrimFunc(pyVersion, func(r rune) bool {
		return !unicode.IsDigit(r) && r != '.'
	})
	appName := project["name"].(string)

	return PyProjectCfg{pyVersion, appName, depList}, nil
}

func intoSource(cfg PyProjectCfg) (*SourceInfo, error) {
	vars := make(map[string]interface{})
	vars["pyVersion"] = cfg.pyVersion
	vars["appName"] = cfg.appName

	if len(cfg.supportedApps) == 0 {
		terminal.Warn("No supported Python frameworks found in your pyproject.toml")
		return nil, nil
	} else if len(cfg.supportedApps) > 1 {
		terminal.Warn("Multiple supported Python frameworks found in your pyproject.toml")
		return nil, nil
	} else if slices.Contains(cfg.supportedApps, FastAPI) {
		return &SourceInfo{
			Files:  templatesExecute("templates/python-fastapi", vars),
			Family: "FastAPI",
			Port:   8000,
		}, nil
	} else if slices.Contains(cfg.supportedApps, Flask) {
		return &SourceInfo{
			Files:  templatesExecute("templates/python-flask-poetry", vars),
			Family: "Flask",
			Port:   8080,
		}, nil
	} else if slices.Contains(cfg.supportedApps, Streamlit) {
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

func configurePyProject(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("pyproject.toml")) {
		return nil, nil
	}
	pyProject, err := readTomlFile("pyproject.toml")
	if err != nil {
		return nil, err
	}
	if checksPass(sourceDir, fileExists("poetry.lock")) {
		cfg, err := fromPoetry(pyProject)
		if err != nil {
			return nil, err
		}
		return intoSource(cfg)

	} else {
		cfg, err := fromPyProject(pyProject)
		if err != nil {
			return nil, err
		}
		return intoSource(cfg)
	}
}

func configurePython(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// using 'poetry.lock' as an indicator instead of 'pyproject.toml', as Paketo doesn't support PEP-517 implementations
	if !checksPass(sourceDir, fileExists("requirements.txt", "environment.yml", "poetry.lock", "Pipfile", "setup.py", "setup.cfg")) {
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
