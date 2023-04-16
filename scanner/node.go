package scanner

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"
)

func configureNode(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	if !checksPass(sourceDir, fileExists("package.json")) {
		return nil, nil
	}

	remix := checksPass(sourceDir, dirContains("package.json", "remix"))
	prisma := checksPass(sourceDir, dirContains("package.json", "prisma"))

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

	if remix {
		s.Family = "Remix"
	}

	if prisma {
		s.Family += "/Prisma"
	}

	vars := make(map[string]interface{})

	var yarnVersion string = "latest"

	// node-build requires a version, so either use the same version as install locally,
	// or default to an LTS version
	var nodeVersion string = "18.15.0"

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

	vars["nodeVersion"] = nodeVersion
	vars["yarnVersion"] = yarnVersion

	vars["remix"] = remix
	vars["prisma"] = prisma

	files := templatesExecute("templates/node", vars)

	// only include migration script if this app uses prisma
	s.Files = make([]SourceFile, 0)
	for _, file := range files {
		if prisma || file.Path != "start_with_migrations.sh" {
			s.Files = append(s.Files, file)
		}
	}

	s.SkipDeploy = true

	scripts, ok := result["scripts"].(map[string]interface{})

	vars["build"] = scripts["build"] != nil

	if !ok || scripts["start"] == nil {
		s.DeployDocs = `
Your Node app doesn't define a start script in package.json.  You will need
to add one before you deploy.  Also be sure to set your listen port
to 8080 using code similar to the following:

    const port = process.env.PORT || "8080";
`
	} else if remix {
		s.DeployDocs = `
Your Remix app is prepared for deployment.
`
	} else {
		s.DeployDocs = `
Your Node app is prepared for deployment.  Be sure to set your listen port
to 8080 using code similar to the following:

    const port = process.env.PORT || "8080";
`
	}

	s.DeployDocs += `
If you need custom packages installed, or have problems with your deployment
build, you may need to edit the Dockerfile for app-specific changes. If you
need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your Node app.
`

	return s, nil
}
