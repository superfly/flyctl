package scanner

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

func configureNode(sourceDir string) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("package.json")) {
		return nil, nil
	}

	data, err := os.ReadFile("package.json")
	if err != nil {
		return nil, nil
	}

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "NodeJS",
		Port:   8080,
		Env: map[string]string{
			"PORT": "8080",
		},
	}

	scripts, ok := result["scripts"].(map[string]interface{})

	if !ok || scripts["start"] == nil {
		// cowardly fall back to heroku buildpacks
		s.Builder = "heroku/buildpacks:20"
		return s, nil
	}

	vars := make(map[string]interface{})

	var nodeVersion string = "latest"
	var yarnVersion string = "latest"

	out, err := exec.Command("node", "-v").Output()

	if err == nil {
		nodeVersion = strings.TrimSpace(string(out))
		if nodeVersion[:1] == "v" {
			nodeVersion = nodeVersion[1:]
		}
	}

	out, err = exec.Command("yarn", "-v").Output()

	if err == nil {
		yarnVersion = strings.TrimSpace(string(out))
	}

	_, err = os.Stat("yarn.lock")
	vars["yarn"] = !os.IsNotExist(err)

	if os.IsNotExist(err) {
		vars["packager"] = "npm"
	} else {
		vars["packager"] = "yarn"
	}

	vars["build"] = scripts["build"] != nil

	vars["nodeVersion"] = nodeVersion
	vars["yarnVersion"] = yarnVersion

	s.Files = templatesExecute("templates/node", vars)

	s.SkipDeploy = true
	s.DeployDocs = `
Your Node app is prepared for deployment.  Be sure to set your listen port
to 8080 using code similar to the following:

    const port = process.env.PORT || "8080";

If you need custom packages installed, or have problems with your deployment
build, you may need to edit the Dockerfile for app-specific changes. If you
need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your Node app.
`

	return s, nil
}
