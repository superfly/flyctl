package cmd

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"time"

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

	fmt.Printf("Heroku App: %s\n", hkApp.Name)
	fmt.Printf("Latest Release: %s\n", latestRelease.ID)
	fmt.Printf("Created: %s\n", latestRelease.CreatedAt.Format(time.RFC3339))
	fmt.Printf("Updated: %s\n", latestRelease.UpdatedAt.Format(time.RFC3339))
	fmt.Printf("Version: %d\n", latestRelease.Version)

	// get the latest release's slug
	slug, err := heroku.SlugInfo(ctx, hkApp.ID, latestRelease.Slug.ID)
	if err != nil {
		return err
	}

	// download the gzipped slug tarball fron slug.Blob.URL write to a temporary file and untar it
	client := new(http.Client)

	req, err := http.NewRequest("GET", slug.Blob.URL, nil)
	if err != nil {
		return err
	}

	req.Header.Set("Accept-Encoding", "gzip")

	res, err := client.Do(req)

	if err != nil {
		return err
	}

	defer res.Body.Close()

	file, err := ioutil.TempFile("", "slug.tar.gz")

	if err != nil {
		return err
	}

	defer os.Remove(file.Name())

	_, err = io.Copy(file, res.Body)
	if err != nil {
		return err
	}

	file.Close()

	if err := os.MkdirAll(app.Name, 0700); err != nil {
		return err
	}

	untar := exec.Command("tar", "-xf", file.Name(), "--strip-components=2", "-C", app.Name)
	untar.Stdout = cmdCtx.IO.Out
	untar.Stderr = cmdCtx.IO.ErrOut

	if err := untar.Run(); err != nil {
		return err
	}

	var procfile string

	for process, command := range slug.ProcessTypes {
		procfile += fmt.Sprintf("%s: %s\n", process, command)
	}

	if err := ioutil.WriteFile(fmt.Sprintf("%s/Procfile", app.Name), []byte(procfile), 0644); err != nil {
		return err
	}

	// retrieve heroku app ENV map[key]value and set it on fly.io as secrets
	env, err := heroku.ConfigVarInfoForApp(ctx, appID)

	if err != nil {
		return err
	}

	if len(env) < 1 {
		fmt.Println("No ENV variables found")
		return nil
	}

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

	return nil
}
