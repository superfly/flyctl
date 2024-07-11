package scanner

import (
	"os"
	"path"
)

func github_actions(sourceDir string, actions *GitHubActionsStruct) {
	// first check to see if the source directory is a git repo that uses github,
	// if so, set secrets and deploy to true
	actions.Secrets = checksPass(sourceDir+"/.git", dirContains("config", "github"))
	actions.Deploy = actions.Secrets

	// See if the source directory is set up to use github actions, if so set deploy to true
	if !actions.Deploy {
		info, err := os.Stat(path.Join(sourceDir, ".github"))
		actions.Deploy = (err == nil && info.IsDir())
	}

	// if deploy is true, add the github actions templates to the files list
	if actions.Deploy {
		actions.Files = templates("templates/github")
	}
}
