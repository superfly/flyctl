package scanner

import (
	"fmt"
	"github.com/mattn/go-zglob"
	"github.com/superfly/flyctl/helpers"
	"path"
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

    wsgis, err := zglob.Glob(`./**/wsgi.py`)

    if err == nil && len(wsgis) > 0 {
        wsgiLen := len(wsgis)
        dirPath, _ := path.Split(wsgis[wsgiLen-1])
        dirName := path.Base(dirPath)
        vars["wsgiName"] = dirName;
        vars["wsgiFound"] = true;
        if wsgiLen > 1 {
            s.DeployDocs = fmt.Sprintf(`
Multiple wsgi.py files were found!

Before proceeding, make sure '%s' is the module containing a WSGI application object named 'application'.

This module is used on Dockerfile to start the Gunicorn server process.
`, dirPath)
        }
    }

    s.Files = templatesExecute("templates/django", vars)

	// check if project has a postgres dependency
	if checksPass(sourceDir, dirContains("requirements.txt", "psycopg")) ||
	    checksPass(sourceDir, dirContains("Pipfile", "psycopg")) ||
	    checksPass(sourceDir, dirContains("pyproject.toml", "psycopg")) {
		s.ReleaseCmd = "python manage.py migrate"

		if !checksPass(sourceDir, dirContains("requirements.txt", "django-environ", "dj-database-url")) {
			s.DeployDocs = s.DeployDocs + `
Your Django app is almost ready to deploy!

We recommend using the django-environ(pip install django-environ) or dj-database-url(pip install dj-database-url) to parse the DATABASE_URL from os.environ['DATABASE_URL']

For detailed documentation, see https://fly.dev/docs/django/
		`
		} else {
			s.DeployDocs = s.DeployDocs + `
Your Django app is ready to deploy!

For detailed documentation, see https://fly.dev/docs/django/
		`
		}
	}

	return s, nil
}
