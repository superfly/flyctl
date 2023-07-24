package launch

import (
	"context"
	"fmt"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
	"github.com/superfly/flyctl/terminal"
)

// Launch launches the app described by the plan. This is the main entry point for launching a plan.
func (state *launchState) Launch(ctx context.Context) error {

	io := iostreams.FromContext(ctx)

	// TODO(Allison): are we still supporting the launch-into usecase?
	// I'm assuming *not* for now, because it's confusing UX and this
	// is the perfect time to remove it.

	state.updateConfig(ctx)

	app, err := state.createApp(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(io.Out, "Created app '%s' in organization '%s'\n", app.Name, app.Organization.Slug)
	fmt.Fprintf(io.Out, "Admin URL: https://fly.io/apps/%s\n", app.Name)
	fmt.Fprintf(io.Out, "Hostname: %s.fly.dev\n", app.Name)

	if err = state.satisfyScannerBeforeDb(ctx); err != nil {
		return err
	}
	dbOptions, err := state.createDatabases(ctx)
	if err != nil {
		return err
	}
	if err = state.satisfyScannerAfterDb(ctx, dbOptions); err != nil {
		return err
	}
	if err = state.createDockerIgnore(ctx); err != nil {
		return err
	}

	// Override internal port if requested using --internal-port flag
	if n := flag.GetInt(ctx, "internal-port"); n > 0 {
		state.appConfig.SetInternalPort(n)
	}

	// Finally write application configuration to fly.toml
	state.appConfig.SetConfigFilePath(state.configPath)
	if err := state.appConfig.WriteToDisk(ctx, state.configPath); err != nil {
		return err
	}

	if state.sourceInfo != nil {
		if err := state.firstDeploy(ctx); err != nil {
			return err
		}
	}

	return nil
}

// updateConfig populates the appConfig with the plan's values
func (state *launchState) updateConfig(ctx context.Context) {
	state.appConfig.AppName = state.plan.AppName
	state.appConfig.PrimaryRegion = state.plan.Region.Code
	if state.plan.Env != nil {
		state.appConfig.SetEnvVariables(state.plan.Env)
	}
}

// createApp creates the fly.io app for the plan
func (state *launchState) createApp(ctx context.Context) (*api.App, error) {
	apiClient := client.FromContext(ctx).API()
	return apiClient.CreateApp(ctx, api.CreateAppInput{
		OrganizationID:  state.plan.Org.ID,
		Name:            state.plan.AppName,
		PreferredRegion: &state.plan.Region.Code,
		Machines:        true,
	})
}

// createDatabases creates databases requested by the plan
func (state *launchState) createDatabases(ctx context.Context) (map[string]bool, error) {
	options := map[string]bool{}

	// TODO: Create databases requested by the plan
	// Base this on v1's createDatabases()

	return options, nil
}

// determineDockerIgnore attempts to create a .dockerignore from .gitignore
func (state *launchState) createDockerIgnore(ctx context.Context) (err error) {
	io := iostreams.FromContext(ctx)
	dockerIgnore := ".dockerignore"
	gitIgnore := ".gitignore"
	allGitIgnores := scanner.FindGitignores(state.workingDir)
	createDockerignoreFromGitignore := false

	// An existing .dockerignore should always be used instead of .gitignore
	if helpers.FileExists(dockerIgnore) {
		terminal.Debugf("Found %s file. Will use when deploying to Fly.\n", dockerIgnore)
		return
	}

	// If we find .gitignore files, determine whether they should be converted to .dockerignore
	if len(allGitIgnores) > 0 {

		if flag.GetBool(ctx, "dockerignore-from-gitignore") {
			createDockerignoreFromGitignore = true
		} else {
			confirm, err := prompt.Confirm(ctx, fmt.Sprintf("Create %s from %d %s files?", dockerIgnore, len(allGitIgnores), gitIgnore))
			if confirm && err == nil {
				createDockerignoreFromGitignore = true
			}
		}

		if createDockerignoreFromGitignore {
			createdDockerIgnore, err := createDockerignoreFromGitignores(state.workingDir, allGitIgnores)
			if err != nil {
				terminal.Warnf("Error creating %s from %d %s files: %v\n", dockerIgnore, len(allGitIgnores), gitIgnore, err)
			} else {
				fmt.Fprintf(io.Out, "Created %s from %d %s files.\n", createdDockerIgnore, len(allGitIgnores), gitIgnore)
			}
			return nil
		}
	}
	return
}
