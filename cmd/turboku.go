package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	hero "github.com/heroku/heroku-go/v5"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/flyctl"
)

var errAppNameTaken = fmt.Errorf("app already exists")

func newTurbokuCommand(client *client.Client) *Command {
	turbokuDocStrings := docstrings.Get("turboku")
	cmd := BuildCommandKS(nil, runTurboku, turbokuDocStrings, client, requireSession)
	cmd.Args = cobra.ExactArgs(1)

	// heroku-token flag
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "heroku-token",
		Description: "Heroku API token",
		EnvName:     "HEROKU_TOKEN",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "now",
		Description: "deploy now without confirmation",
		Default:     false,
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "org",
		Description: `the organization that will own the app`,
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "name",
		Description: "the name of the new app",
	})

	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "keep",
		Description: "keep the app directory after deployment",
		Default:     false,
	})

	cmd.AddStringFlag(StringFlagOpts{
		Name:        "region",
		Description: "the region to launch the new app in",
	},
	)

	return cmd
}

// runTurboku fetches a heroku app and creates it on fly.io
func runTurboku(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	dir := cmdCtx.Config.GetString("path")

	if absDir, err := filepath.Abs(dir); err == nil {
		dir = absDir
	}
	cmdCtx.WorkingDir = dir

	fly := cmdCtx.Client.API()

	var herokuAppName string

	// get heroku token
	herokuToken := cmdCtx.Config.GetString("heroku-token")
	if herokuToken == "" {
		return fmt.Errorf("heroku-token is required")
	}

	hero.DefaultTransport.BearerToken = herokuToken

	appID := cmdCtx.Args[0]

	heroku := hero.NewService(hero.DefaultClient)

	hkApp, err := heroku.AppInfo(ctx, appID)
	if err != nil {
		return err
	}

	// print the heroku app name we are using
	fmt.Printf("Using heroku app: %s\n", hkApp.Name)

	// Heroku regions are in Virigina (US) and Ireland (EU), so use the closest datacenters
	var regionCode string

	if code := cmdCtx.Config.GetString("region"); code != "" {
		region, err := selectRegion(ctx, fly, code)
		if err != nil {
			return err
		}
		regionCode = region.Code
	} else {
		if hkApp.Region.Name == "us" {
			regionCode = "iad"
		} else {
			regionCode = "lhr"
		}
	}

	fmt.Printf("Selected fly region: %s\n", regionCode)

	if len(cmdCtx.Args) > 0 {
		herokuAppName = cmdCtx.Args[0]
	}

	var appName string

	if appName = cmdCtx.Config.GetString("name"); appName == "" {

		inputName, err := inputAppName(herokuAppName, false)
		if err != nil {
			return err
		}
		appName = inputName
	}

	orgSlug := cmdCtx.Config.GetString("org")

	org, err := selectOrganization(ctx, fly, orgSlug)
	if err != nil {
		return err
	}

	input := api.CreateAppInput{
		Name:            appName,
		OrganizationID:  org.ID,
		PreferredRegion: api.StringPointer(regionCode),
	}

	app, err := fly.CreateApp(ctx, input)

	switch isTakenError(err) {

	case nil:
		fmt.Printf("New app created: %s\n", app.Name)

	case errAppNameTaken:
		fmt.Printf("App %s already exists\n", appName)

		app, err = fly.GetApp(ctx, appName)
		if err != nil {
			return err
		}
	default:
		return err
	}

	// retrieve heroku app ENV map[key]value and set it on fly.io as secrets
	env, err := heroku.ConfigVarInfoForApp(ctx, appID)
	if err != nil {
		return err
	}

	if len(env) >= 1 {
		// add the env map[key]value items to a secrets map[key]value
		secrets := make(map[string]string)

		for key, value := range env {
			secrets[key] = *value
		}

		_, err = fly.SetSecrets(ctx, app.Name, secrets)
		if err != nil {
			if !strings.Contains(err.Error(), "No change") {
				return err
			}
		}

		if !app.Deployed {
			cmdCtx.Statusf("secrets", cmdctx.SINFO, "Secrets are staged for the first deployment\n")
		} else {
			cmdCtx.Statusf("secrets", cmdctx.SINFO, "Secrets are deployed\n")
		}
	}

	// get latest release
	releases, err := heroku.ReleaseList(ctx, appID, &hero.ListRange{Field: "version", Descending: true, Max: 1})
	if err != nil {
		return err
	}

	if len(releases) == 0 {
		fmt.Println("No releases found")
		return nil
	}

	latestRelease := releases[0]

	// get the latest release's slug
	slug, err := heroku.SlugInfo(ctx, hkApp.ID, latestRelease.Slug.ID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(app.Name, 0o750); err != nil {
		return err
	}

	// Generate an app config to write to fly.toml
	appConfig := flyctl.NewAppConfig()

	appConfig.Definition = app.Config.Definition
	procfile := ""

	// Add each process to a Procfile and fly.toml
	for process, command := range slug.ProcessTypes {
		if process == "release" {
			appConfig.SetReleaseCommand(command)
		} else if process != "console" && process != "rake" {
			procfile += fmt.Sprintf("%s: %s\n", process, command)

			// 'app' is the default process in our config
			if process == "web" {
				process = "app"
			}

			appConfig.SetProcess(process, command)
		}
	}

	if err := ioutil.WriteFile(fmt.Sprintf("%s/Procfile", app.Name), []byte(procfile), 0o644); err != nil {
		return err
	}

	fmt.Printf("Procfile created: %s/Procfile\n", app.Name)

	if err := createDockerfile(app.Name, slug.Stack.Name, slug.Blob.URL); err != nil {
		return err
	}

	fmt.Printf("Dockerfile created: %s/Dockerfile\n", app.Name)

	cmdCtx.AppName = app.Name
	appConfig.AppName = app.Name
	cmdCtx.AppConfig = appConfig

	// Write the app config
	if err := writeAppConfig(filepath.Join(app.Name, "fly.toml"), appConfig); err != nil {
		return err
	}

	if !cmdCtx.Config.GetBool("no-deploy") && (cmdCtx.Config.GetBool("now") || confirm("Would you like to deploy now?")) {
		// change working directory(cmdCtx.WorkingDir) to the app directory
		cmdCtx.WorkingDir = filepath.Join(cmdCtx.WorkingDir, app.Name)

		// runDeploy
		if err := runDeploy(cmdCtx); err != nil {
			return err
		}

		fmt.Printf("App deployed: %s\n", app.Name)

		if !cmdCtx.Config.GetBool("keep") {
			if err := os.RemoveAll(app.Name); err != nil {
				return err
			}
		}
	}

	return nil
}

func createDockerfile(appName, baseImage, slugURL string) error {
	baseImage = fmt.Sprintf("%s/%s", "heroku", strings.Replace(baseImage, "-", ":", 1))

	entrypoint := `
for f in /app/.profile.d/*.sh; do . $f; done
eval "exec $@"
`
	ioutil.WriteFile(fmt.Sprintf("%s/entrypoint.sh", appName), []byte(entrypoint), 0o6750)

	dockerfileTemplate := `FROM %s
RUN useradd -m heroku
RUN mkdir /app
WORKDIR /app
ENV HOME /app
ENV PORT 8080
COPY Procfile /app
COPY entrypoint.sh /app
ENTRYPOINT ["/bin/bash", "/app/entrypoint.sh"]

RUN curl "%s" | tar xzf - --strip 2 -C /app`

	dockerfile := fmt.Sprintf(dockerfileTemplate, baseImage, slugURL)
	dockerfile += "\nRUN chown -R heroku:heroku /app\n"
	dockerfile += "\nUSER heroku\n"

	return ioutil.WriteFile(fmt.Sprintf("%s/Dockerfile", appName), []byte(dockerfile), 0o640)
}

func isTakenError(err error) error {
	if err != nil && strings.Contains(err.Error(), "taken") {
		return errAppNameTaken
	}
	return err
}
