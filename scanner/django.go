package scanner

import (
	"github.com/superfly/flyctl/helpers"
	"os"
	"path/filepath"
)

// setup django with a postgres database
func configureDjango(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("requirements.txt", "(?i)Django")) && !checksPass(sourceDir, dirContains("Pipfile", "(?i)Django")) && !checksPass(sourceDir, dirContains("pyproject.toml", "(?i)Django")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Django",
		Port:   8000,
		Env: map[string]string{
			"PORT": "8000",
		},
		Secrets: []Secret{
			{
				Key:  "SECRET_KEY",
				Help: "Django needs a random, secret key. Use the random default we've generated, or generate your own.",
				Generate: func() (string, error) {
					return helpers.RandString(64)
				},
			},
		},
		Statics: []Static{
			{
				GuestPath: "/code/static",
				UrlPrefix: "/static/",
			},
		},
		SkipDeploy: true,
	}

	vars := make(map[string]interface{})

    if checksPass(sourceDir, fileExists("Pipfile")) {
	    vars["pipenv"] = true
    } else if checksPass(sourceDir, fileExists("pyproject.toml")) {
	    vars["poetry"] = true
	} else if checksPass(sourceDir, fileExists("requirements.txt")) {
	    vars["venv"] = true
	}

    fileName := "wsgi.py"
    root := "."

    // Walk the directory tree and search for the wsgi.py file
    var filePath string
    err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
        if err != nil {
            return err
        }
        if !info.IsDir() && info.Name() == fileName {
            filePath = path
        }
        return nil
    })

    if err != nil || filePath == "" {
        vars["wsgiFound"] = false;
    } else {
        vars["wsgiFound"] = true;
        vars["wsgiName"] = filepath.Base(filepath.Dir(filePath));
    }

    s.Files = templatesExecute("templates/django", vars)

	// check if project has a postgres dependency
	if checksPass(sourceDir, dirContains("requirements.txt", "psycopg2")) || checksPass(sourceDir, dirContains("Pipfile", "psycopg2")) || checksPass(sourceDir, dirContains("pyproject.toml", "psycopg2")) {
		s.ReleaseCmd = "python manage.py migrate"

		if !checksPass(sourceDir, dirContains("requirements.txt", "django-environ", "dj-database-url")) {
			s.DeployDocs = `
Your Django app is almost ready to deploy!

We recommend using the django-environ(pip install django-environ) or dj-database-url(pip install dj-database-url) to parse the DATABASE_URL from os.environ['DATABASE_URL']

For detailed documentation, see https://fly.dev/docs/django/
		`
		} else {
			s.DeployDocs = `
Your Django app is ready to deploy!

For detailed documentation, see https://fly.dev/docs/django/
		`
		}
	}

	return s, nil
}
