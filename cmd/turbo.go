package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	hero "github.com/heroku/heroku-go/v5"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/client"
)

func newTurboCommand(client *client.Client) *Command {
	turboDocStrings := docstrings.Get("turbo")
	cmd := BuildCommandKS(nil, runTurbo, turboDocStrings, client, requireSession)
	cmd.Args = cobra.ExactArgs(1)

	// heroku-token flag
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "heroku-token",
		Description: "Heroku API token",
		EnvName:     "HEROKU_TOKEN",
	})
	return cmd
}

// runTurbo fetches a heroku app and creates it on fly.io
func runTurbo(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	fly := cmdCtx.Client.API()

	// get the app name
	appName := cmdCtx.Args[0]

	orgSlug := cmdCtx.Config.GetString("org")

	org, err := selectOrganization(ctx, fly, orgSlug, nil)
	if err != nil {
		return err
	}

	input := api.CreateAppInput{
		Name:           appName,
		Runtime:        "FIRECRACKER",
		OrganizationID: org.ID,
	}

	app, err := fly.CreateApp(ctx, input)
	if err != nil {
		return err
	}

	fmt.Printf("New app created: %s\n", app.Name)

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
			return err
		}

		if !app.Deployed {
			cmdCtx.Statusf("secrets", cmdctx.SINFO, "Secrets are staged for the first deployment\n")
			return nil
		}

		cmdCtx.Statusf("secrets", cmdctx.SINFO, "Secrets are deployed\n")
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

	if err := os.MkdirAll(app.Name, 0755); err != nil {
		return err
	}

	procfile := ""
	for process, command := range slug.ProcessTypes {
		if process != "release" {
			procfile += fmt.Sprintf("%s: %s\n", process, command)
		}
	}

	if err := ioutil.WriteFile(fmt.Sprintf("%s/Procfile", app.Name), []byte(procfile), 0644); err != nil {
		return err
	}

	fmt.Printf("Procfile created: %s/Procfile\n", app.Name)

	if err := createDockerfile(app.Name, slug.Stack.Name, slug.Blob.URL); err != nil {
		return err
	}

	fmt.Printf("Dockerfile created: %s/Dockerfile\n", app.Name)

	return nil
}

func createDockerfile(appName, baseImage, slugURL string) error {
	baseImage = fmt.Sprintf("%s/%s", "heroku", strings.Replace(baseImage, "-", ":", 1))

	dockerfile := fmt.Sprintf(`FROM %s
RUN mkdir /app
WORKDIR /app
RUN curl %s | tar xzvf -`, baseImage, slugURL)

	dockerfile += "\n"

	dockerfile += "ADD Procfile /app\n"

	return ioutil.WriteFile(fmt.Sprintf("%s/Dockerfile", appName), []byte(dockerfile), 0644)
}
