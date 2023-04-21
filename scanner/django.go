package scanner

import (
	"fmt"
	"github.com/mattn/go-zglob"
	"github.com/superfly/flyctl/helpers"
	"path"
	"strings"
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
            // warning: multiple wsgi.py files found
            s.DeployDocs = s.DeployDocs + fmt.Sprintf(`
Multiple wsgi.py files were found!
Before proceeding, make sure '%s' is the module containing a WSGI application object named 'application'.
This module is used on Dockerfile to start the Gunicorn server process.
`, dirPath)
        }
    }

    // check if settings.py file exists
    settingsFiles, err := zglob.Glob(`./**/settings.py`)

    if err == nil && len(settingsFiles) == 0 {
        // if no settings.py files are found, check if any *prod*.py (e.g. production.py, prod.py, settings_prod.py) exists in 'settings/' folder
        settingsFiles, err = zglob.Glob(`./**/settings/*prod*.py`)
    }

    if err == nil && len(settingsFiles) > 0 {
        settingsFilesLen := len(settingsFiles)
        // check if multiple settings.py files were found; warn the user it's not recommended and what to do instead
        if settingsFilesLen > 1 {
            // warning: multiple settings.py files found
            s.DeployDocs = s.DeployDocs + fmt.Sprintf(`
Multiple 'settings.py' files were found in your Django application:
[%s]
It's not recommended to have multiple 'settings.py' files.
Instead, you can have a 'settings/' folder with the settings files according to the different environments (e.g., local.py, staging.py, production.py).
In this case, you can specify which settings file to use when running the Django application by setting the 'DJANGO_SETTINGS_MODULE' environment variable to the corresponding settings file.
`, strings.Join(settingsFiles, ", "))
        }
        // check if STATIC_ROOT setting is set in ANY of the settings.py files
        for _, settingsPath := range settingsFiles {
            // in production, you must define a STATIC_ROOT directory where collectstatic will copy them.
            if checksPass(sourceDir, dirContains(settingsPath, "STATIC_ROOT")) {
                vars["collectStatic"] = true
                s.DeployDocs = s.DeployDocs + fmt.Sprintf(`
'STATIC_ROOT' setting was detected in '%s'!
Static files will be collected during build time by running 'python manage.py collectstatic' on Dockerfile.
`, settingsPath)
                // check if django.core.management.utils.get_random_secret_key() is used to set a default secret key
                // if not found, set a random SECRET_KEY for building purposes
                if checksPass(sourceDir, dirContains(settingsPath, "default=get_random_secret_key()")) {
                    vars["hasRandomSecretKey"] = true
                } else {
                    // generate a random 50 character random string usable as a SECRET_KEY setting value on Dockerfile
                    // based on https://github.com/django/django/blob/main/django/core/management/utils.py#L79
                    randomSecretKey, err := helpers.RandString(50)
                    if err == nil {
                        vars["randomSecretKey"] = randomSecretKey
                        s.DeployDocs = s.DeployDocs + fmt.Sprintf(`
A default SECRET_KEY was not detected in '%s'!
A generated SECRET_KEY "%s" was set on Dockerfile for building purposes.
Optionally, you can use django.core.management.utils.get_random_secret_key() to set the SECRET_KEY default value in your %s.
`, settingsPath, randomSecretKey, settingsPath)
                    }
                }
                break
            }
        }
    }

    s.Files = templatesExecute("templates/django", vars)

	// check if project has a postgres dependency
	if checksPass(sourceDir, dirContains("requirements.txt", "psycopg2")) || checksPass(sourceDir, dirContains("Pipfile", "psycopg2")) || checksPass(sourceDir, dirContains("pyproject.toml", "psycopg2")) {
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
