package scanner

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/superfly/flyctl/helpers"
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

	var packageJson map[string]interface{}
	err = json.Unmarshal(data, &packageJson)
	if err != nil {
		return nil, nil
	}

	s := &SourceInfo{
		Family: "NodeJS",
		Port:   8080,
	}

	env := map[string]string{
		"PORT": "8080",
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
	var nodeLtsVersion string = "18.16.0"
	var nodeVersion string = nodeLtsVersion

	out, err := exec.Command("node", "-v").Output()

	if err == nil {
		nodeVersion = strings.TrimSpace(string(out))
		if nodeVersion[:1] == "v" {
			nodeVersion = nodeVersion[1:]
		}
		if nodeVersion < "16" {
			s.Notice += fmt.Sprintf("\n[WARNING] It looks like you have NodeJS v%s installed, but it has reached it's end of support. Using NodeJS v%s (LTS) to build your image instead.\n", nodeVersion, nodeLtsVersion)
			nodeVersion = nodeLtsVersion
		}
	}

	out, err = exec.Command("yarn", "-v").Output()

	if err == nil {
		yarnVersion = strings.TrimSpace(string(out))
	}

	package_files := []string{"package.json"}

	_, err = os.Stat("yarn.lock")
	vars["yarn"] = !os.IsNotExist(err)

	if os.IsNotExist(err) {
		vars["packager"] = "npm"

		_, err = os.Stat("package-lock.json")
		if !os.IsNotExist(err) {
			package_files = append(package_files, "package-lock.json")
		}
	} else {
		vars["packager"] = "yarn"
		package_files = append(package_files, "yarn.lock")
	}

	vars["nodeVersion"] = nodeVersion
	vars["yarnVersion"] = yarnVersion
	vars["package_files"] = strings.Join(package_files, " ")

	vars["remix"] = remix
	vars["prisma"] = prisma

	vars["runtime"] = s.Family

	if checksPass(sourceDir+"/prisma", dirContains("*.prisma", "sqlite")) {
		env["DATABASE_URL"] = "file:/data/sqlite.db"
		s.SkipDatabase = true
		s.Volumes = []Volume{
			{
				Source:      "data",
				Destination: "/data",
			},
		}
		s.Notice += "\nThis launch configuration uses SQLite on a single, dedicated volume. It will not scale beyond a single VM. Look into 'fly postgres' for a more robust production database."
	}

	if remix {
		bytes, err := helpers.RandBytes(32)

		if err == nil {
			s.Secrets = []Secret{
				{
					Key:   "SESSION_SECRET",
					Help:  "Secret key used to verify the integrity of signed cookies",
					Value: hex.EncodeToString(bytes),
				},
			}

		}
	}

	s.SkipDeploy = true

	vars["devDependencies"] = packageJson["devDependencies"] != nil

	scripts, ok := packageJson["scripts"].(map[string]interface{})

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

	files := templatesExecute("templates/node", vars)

	// only include migration script if this app uses prisma
	s.Files = make([]SourceFile, 0)
	for _, file := range files {
		if prisma || file.Path != "docker-entrypoint" {
			s.Files = append(s.Files, file)
		}
	}

	s.Env = env

	return s, nil
}
