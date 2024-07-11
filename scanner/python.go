package scanner

import (
	"bufio"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/pkg/errors"

	"github.com/pelletier/go-toml/v2"
	"github.com/superfly/flyctl/terminal"
)

type PyDepStyle string

const (
	Poetry PyDepStyle = "poetry"
	Pipenv PyDepStyle = "pipenv"
	Pep621 PyDepStyle = "pep621"
	Pip    PyDepStyle = "pip"
)

type PyApp string

const (
	FastAPI   PyApp = "fastapi"
	Flask     PyApp = "flask"
	Streamlit PyApp = "streamlit"
)

var supportedApps = []PyApp{FastAPI, Flask, Streamlit}

type PyProjectToml struct {
	Project struct {
		Name           string
		Version        string
		Dependencies   []string
		RequiresPython string `toml:"requires-python"`
	}
	Tool struct {
		Poetry struct {
			Name         string
			Version      string
			Dependencies map[string]interface{}
		}
	}
}

type Pipfile struct {
	Packages map[string]interface{}
}

type PyCfg struct {
	pyVersion string
	appName   string
	deps      []string
	depStyle  PyDepStyle
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

func readLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	file.Close()
	return lines, nil
}

func intoSource(cfg PyCfg) (*SourceInfo, error) {
	vars := make(map[string]interface{})
	vars["pyVersion"] = cfg.pyVersion
	vars["appName"] = cfg.appName
	var app PyApp
	for _, dep := range cfg.deps {
		if slices.Contains(supportedApps, PyApp(dep)) && app == "" {
			app = PyApp(dep)
		} else if slices.Contains(supportedApps, PyApp(dep)) && app != "" {
			terminal.Warn("Multiple supported Python frameworks found")
			return nil, nil
		}
	}
	vars[string(cfg.depStyle)] = true
	objectStorage := slices.Contains(cfg.deps, "boto3") || slices.Contains(cfg.deps, "boto")
	if app == "" {
		terminal.Warn("No supported Python frameworks found")
		return nil, nil
	} else if app == FastAPI {
		vars["fastapi"] = true
		return &SourceInfo{
			Files:                templatesExecute("templates/python-docker", vars),
			Family:               "FastAPI",
			Port:                 8000,
			ObjectStorageDesired: objectStorage,
		}, nil
	} else if app == Flask {
		vars["flask"] = true
		return &SourceInfo{
			Files:                templatesExecute("templates/python-docker", vars),
			Family:               "Flask",
			Port:                 8080,
			ObjectStorageDesired: objectStorage,
		}, nil
	} else if app == Streamlit {
		vars["streamlit"] = true
		entrypoint := findEntrypoint("streamlit")
		if entrypoint == nil {
			return nil, nil
		} else {
			vars["entrypoint"] = entrypoint.Name()
		}
		return &SourceInfo{
			Files:                templatesExecute("templates/python-docker", vars),
			Family:               "Streamlit",
			Port:                 8501,
			ObjectStorageDesired: objectStorage,
		}, nil
	} else {
		return nil, nil
	}
}

func configPoetry(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("poetry.lock")) || !checksPass(sourceDir, fileExists("pyproject.toml")) {
		return nil, nil
	}
	terminal.Info("Detected Poetry project")
	doc, err := os.ReadFile("pyproject.toml")
	if err != nil {
		return nil, errors.Wrap(err, "Error reading pyproject.toml")
	}

	var pyProject PyProjectToml
	if err := toml.Unmarshal(doc, &pyProject); err != nil {
		return nil, errors.Wrap(err, "Error parsing pyproject.toml")
	}
	deps := pyProject.Tool.Poetry.Dependencies
	appName := pyProject.Tool.Poetry.Name

	if deps == nil {
		return nil, errors.New("No dependencies found in pyproject.toml")
	}
	var depList []string

	for dep := range deps {
		depList = append(depList, parsePyDep(dep))
	}
	pyVersion := deps["python"].(string)
	pyVersion = strings.TrimPrefix(pyVersion, "^")
	pyVersion = parsePyDep(pyVersion)
	cfg := PyCfg{pyVersion, appName, depList, Poetry}
	return intoSource(cfg)
}

func configPyProject(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("pyproject.toml")) {
		return nil, nil
	}
	terminal.Info("Detected pyproject.toml")
	doc, err := os.ReadFile("pyproject.toml")
	if err != nil {
		return nil, errors.Wrap(err, "Error reading pyproject.toml")
	}
	var pyProject PyProjectToml
	if err := toml.Unmarshal(doc, &pyProject); err != nil {
		return nil, errors.Wrap(err, "Error parsing pyproject.toml")
	}
	deps := pyProject.Project.Dependencies
	if deps == nil {
		return nil, errors.New("No dependencies found in pyproject.toml")
	}
	var depList []string
	for _, dep := range deps {
		dep := parsePyDep(dep)
		depList = append(depList, parsePyDep(dep))
	}
	appName := pyProject.Project.Name
	pyVersion := pyProject.Project.RequiresPython
	if pyVersion == "" {
		extracted, _, err := extractPythonVersion()
		if err != nil {
			return nil, err
		}
		pyVersion = extracted
	} else {
		pyVersion = strings.TrimFunc(pyVersion, func(r rune) bool {
			return !unicode.IsDigit(r) && r != '.'
		})
	}

	cfg := PyCfg{pyVersion, appName, depList, Pep621}
	return intoSource(cfg)
}

func configPipfile(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("Pipfile", "Pipfile.lock")) {
		return nil, nil
	}
	terminal.Info("Detected Pipfile")
	doc, err := os.ReadFile("Pipfile")
	if err != nil {
		return nil, errors.Wrap(err, "Error reading Pipfile")
	}
	var pipfile Pipfile
	if err := toml.Unmarshal(doc, &pipfile); err != nil {
		return nil, errors.Wrap(err, "Error parsing Pipfile")
	}
	deps := pipfile.Packages
	if deps == nil {
		return nil, errors.New("No packages found in Pipfile")
	}
	var depList []string
	for dep := range deps {
		dep := parsePyDep(dep)
		depList = append(depList, dep)
	}
	pyVersion, _, err := extractPythonVersion()
	if err != nil {
		return nil, err
	}
	appName := filepath.Base(sourceDir)
	cfg := PyCfg{pyVersion, appName, depList, Pipenv}
	return intoSource(cfg)
}

func configRequirements(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	var deps []string
	if checksPass(sourceDir, fileExists("requirements.txt")) {
		terminal.Info("Detected requirements.txt")
		req_deps, err := readLines("requirements.txt")
		if err != nil {
			return nil, err
		}
		deps = req_deps
	} else if checksPass(sourceDir, fileExists("requirements.in")) {
		terminal.Info("Detected requirements.in")
		req_deps, err := readLines("requirements.in")
		if err != nil {
			return nil, err
		}
		deps = req_deps
	} else {
		return nil, nil
	}
	if deps == nil {
		return nil, errors.New("No dependencies found in requirements file")
	}
	var depList []string
	for _, dep := range deps {
		dep := parsePyDep(dep)
		depList = append(depList, dep)
	}
	pyVersion, _, err := extractPythonVersion()
	if err != nil {
		return nil, err
	}
	appName := filepath.Base(sourceDir)
	cfg := PyCfg{pyVersion, appName, depList, Pip}
	return intoSource(cfg)
}

func configurePython(sourceDir string, _ *ScannerConfig) (*SourceInfo, error) {
	src, err := configPoetry(sourceDir, nil)
	if src != nil || err != nil {
		return src, err
	}
	src, err = configPyProject(sourceDir, nil)
	if src != nil || err != nil {
		return src, err
	}
	src, err = configPipfile(sourceDir, nil)
	if src != nil || err != nil {
		return src, err
	}
	src, err = configRequirements(sourceDir, nil)
	if src != nil || err != nil {
		return src, err
	}
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
