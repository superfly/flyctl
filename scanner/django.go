package scanner

import (
	"context"

	"github.com/superfly/flyctl/helpers"
)

// setup django with a postgres database
func configureDjango(ctx context.Context, sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, dirContains("requirements.txt", "Django")) {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "Django",
		Port:   8000,
		Files:  templates("templates/django"),
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
				GuestPath: "/app/public",
				UrlPrefix: "/static/",
			},
		},
		SkipDeploy: true,
	}

	// check if requirements.txt has a postgres dependency
	if checksPass(sourceDir, dirContains("requirements.txt", "psycopg2")) {
		s.InitCommands = []InitCommand{
			{
				// python makemigrations
				Command:     "python",
				Args:        []string{"manage.py", "makemigrations"},
				Description: "Creating database migrations",
			},
		}
		s.ReleaseCmd = "python manage.py migrate"

		if !checksPass(sourceDir, dirContains("requirements.txt", "database_url")) {
			s.DeployDocs = `
Your Django app is almost ready to deploy!

We recommend using the database_url(pip install dj-database-url) to parse the DATABASE_URL from os.environ['DATABASE_URL']

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
