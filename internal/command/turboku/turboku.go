package turboku

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	hero "github.com/heroku/heroku-go/v5"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/internal/app"
	"github.com/superfly/flyctl/internal/command"
	"github.com/superfly/flyctl/internal/command/deploy"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/heroku"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
)

var errAppNameTaken = fmt.Errorf("app already exists")

func New() (cmd *cobra.Command) {

	const (
		long  = `Launch a Heroku app on Fly.io`
		short = long
		usage = "turboku <heroku-app-name> <heroku-api-token>"
	)

	cmd = command.New(usage, short, long, run, command.RequireSession)

	flag.Add(cmd,
		flag.Org(),
		flag.Region(),
		flag.Now(),
		flag.NoDeploy(),

		flag.Bool{
			Name:        "keep",
			Description: "keep the app directory after deployment",
			Default:     false,
		},
		flag.String{
			Name:        "name",
			Description: "the name of the new app",
		},
	)
	cmd.Args = cobra.MinimumNArgs(2)
	return cmd
}

// run fetches a heroku app and creates it on fly.io
func run(ctx context.Context) error {

	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)

	herokuAppName := flag.FirstArg(ctx)

	herokuClient := heroku.New(flag.Args(ctx)[1])

	hkApp, err := herokuClient.AppInfo(ctx, herokuAppName)
	if err != nil {
		return err
	}

	// print the heroku app name we are using
	fmt.Fprintf(io.Out, "Using heroku app: %s\n", hkApp.Name)

	// Heroku regions are in Virigina (US) and Ireland (EU), so use the closest datacenters
	var regionCode string

	if regionCode = flag.GetString(ctx, "region"); regionCode == "" {
		if hkApp.Region.Name == "us" {
			regionCode = "iad"
		} else {
			regionCode = "lhr"
		}
	}

	fmt.Fprintf(io.Out, "Selected fly region: %s\n", regionCode)

	var flyAppName string

	if flyAppName = flag.GetString(ctx, "name"); flyAppName == "" {

		inputName, err := prompt.SelectAppName(ctx)
		if err != nil {
			return err
		}
		flyAppName = inputName
	}

	org, err := prompt.Org(ctx)

	if err != nil {
		return err
	}

	input := api.CreateAppInput{
		Name:            flyAppName,
		OrganizationID:  org.ID,
		PreferredRegion: api.StringPointer(regionCode),
	}

	createdApp, err := client.CreateApp(ctx, input)

	switch isTakenError(err) {

	case nil:
		fmt.Printf("New app created: %s\n", createdApp.Name)

	case errAppNameTaken:
		fmt.Printf("App %s already exists\n", flyAppName)

		createdApp, err = client.GetApp(ctx, flyAppName)
		if err != nil {
			return err
		}
	default:
		return err
	}

	// retrieve heroku app ENV map[key]value and set it on fly.io as secrets
	env, err := herokuClient.ConfigVarInfoForApp(ctx, herokuAppName)
	if err != nil {
		return err
	}

	if len(env) >= 1 {
		// add the env map[key]value items to a secrets map[key]value
		secrets := make(map[string]string)

		for key, value := range env {
			secrets[key] = *value
		}

		_, err = client.SetSecrets(ctx, createdApp.Name, secrets)
		if err != nil {
			if !strings.Contains(err.Error(), "No change") {
				return err
			}
		}

		if !createdApp.Deployed {
			fmt.Fprintf(io.Out, "Secrets are staged for the first deployment\n")
		} else {
			fmt.Fprintf(io.Out, "Secrets are deployed\n")
		}
	}

	// get latest release
	releases, err := herokuClient.ReleaseList(ctx, herokuAppName, &hero.ListRange{Field: "version", Descending: true, Max: 1})
	if err != nil {
		return err
	}

	if len(releases) == 0 {
		fmt.Println("No releases found")
		return nil
	}

	latestRelease := releases[0]

	// get the latest release's slug
	slug, err := herokuClient.SlugInfo(ctx, hkApp.ID, latestRelease.Slug.ID)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(createdApp.Name, 0o750); err != nil {
		return err
	}

	ctx, err = command.ChangeWorkingDirectory(ctx, createdApp.Name)

	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Changed to new app directory %s\n", createdApp.Name)

	// Generate an app config to write to fly.toml
	appConfig := app.NewConfig()

	appConfig.Definition = createdApp.Config.Definition
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

	if err := ioutil.WriteFile("Procfile", []byte(procfile), 0o644); err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "Procfile created")

	if err := createDockerfile(createdApp.Name, slug.Stack.Name, slug.Blob.URL); err != nil {
		return err
	}

	fmt.Fprintln(io.Out, "Dockerfile created")

	appConfig.AppName = createdApp.Name

	// Write the app config
	if err = appConfig.WriteToDisk(ctx, "fly.toml"); err != nil {
		return err
	}

	deployNow := false
	promptForDeploy := true

	if flag.GetBool(ctx, "no-deploy") {
		deployNow = false
		promptForDeploy = false
	}

	if flag.GetBool(ctx, "now") {
		deployNow = true
		promptForDeploy = false
	}

	if promptForDeploy {
		confirm, err := prompt.Confirm(ctx, "Would you like to deploy now?")
		if confirm && err == nil {
			deployNow = true
		}
	}

	if deployNow {
		if !flag.GetBool(ctx, "keep") {
			if err := os.RemoveAll(createdApp.Name); err != nil {
				return err
			}
		}
		return deploy.DeployWithConfig(ctx, appConfig, deploy.DeployWithConfigArgs{
			ForceNomad: true,
			ForceYes:   deployNow,
		})
	}

	return nil
}

func createDockerfile(appName, baseImage, slugURL string) error {
	baseImage = fmt.Sprintf("%s/%s", "heroku", strings.Replace(baseImage, "-", ":", 1))

	entrypoint := `
for f in /app/.profile.d/*.sh; do . $f; done
eval "exec $@"
`
	ioutil.WriteFile("entrypoint.sh", []byte(entrypoint), 0o6750)

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

	return ioutil.WriteFile("Dockerfile", []byte(dockerfile), 0o640)
}

func isTakenError(err error) error {
	if err != nil && strings.Contains(err.Error(), "taken") {
		return errAppNameTaken
	}
	return err
}
