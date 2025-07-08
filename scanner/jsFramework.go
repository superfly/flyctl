package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/superfly/flyctl/internal/command/launch/plan"
)

var packageJson map[string]interface{}

// Handle js frameworks separate from other node applications.  Currently the requirements
// for a framework is pretty low: to have a "start" script.  Because we are actually
// going to be running a js application to generate a Dockerfile there is one more
// criteria: if you are running node, the running node version must be at least 16,
// for bun the running bun version must be at least 0.5.3.  If there turns out to be
// demand for earlier versions of node or bun, we can adjust this requirement.
func configureJsFramework(sourceDir string, config *ScannerConfig) (*SourceInfo, error) {
	// first ensure that there is a package.json
	if !checksPass(sourceDir, fileExists("package.json")) {
		return nil, nil
	}

	// ensure package.json has a main, module, or start script
	data, err := os.ReadFile("package.json")

	if err != nil {
		return nil, nil
	} else {
		err = json.Unmarshal(data, &packageJson)
		if err != nil {
			return nil, nil
		}

		// see if package.json has a "main"
		main, _ := packageJson["main"].(string)

		// check for tyep="module" and a module being defined
		ptype, ok := packageJson["type"].(string)
		if ok && ptype == "module" {
			module, ok := packageJson["type"].(string)
			if ok {
				main = module
			}
		}

		// check for a start script
		scripts, ok := packageJson["scripts"].(map[string]interface{})

		if ok && scripts["start"] != nil {
			start, ok := scripts["start"].(string)
			if ok {
				main = start
			}
		}

		// bail if no entrypoint can be found
		if main == "" {
			return nil, nil
		}
	}

	srcInfo := &SourceInfo{
		Family:   "NodeJS",
		Callback: JsFrameworkCallback,
	}

	_, err = os.Stat("bun.lockb")
	if errors.Is(err, fs.ErrNotExist) {
		// ensure node is in $PATH
		node, err := exec.LookPath("node")
		if err != nil && !errors.Is(err, exec.ErrDot) {
			return nil, nil
		}

		// resolve to absolute path, see: https://tip.golang.org/doc/go1.19#os-exec-path
		node, err = filepath.Abs(node)
		if err != nil {
			return nil, nil
		}

		// ensure node version is at least 16.0.0
		out, err := exec.Command(node, "-v").Output()
		if err != nil {
			return nil, nil
		} else {
			minVersion, err := semver.Make("16.0.0")
			if err != nil {
				panic(err)
			}

			nodeVersionString := strings.TrimSpace(string(out))
			if nodeVersionString[:1] == "v" {
				nodeVersionString = nodeVersionString[1:]
			}

			nodeVersion, err := semver.Make(nodeVersionString)

			if err != nil || nodeVersion.LT(minVersion) {
				return nil, nil
			}

			srcInfo.Runtime = plan.RuntimeStruct{Language: "node", Version: nodeVersionString}
		}
	} else {
		// ensure bun is in $PATH
		bun, err := exec.LookPath("bun")
		if err != nil && !errors.Is(err, exec.ErrDot) {
			return nil, nil
		}

		// resolve to absolute path, see: https://tip.golang.org/doc/go1.19#os-exec-path
		bun, err = filepath.Abs(bun)
		if err != nil {
			return nil, nil
		}

		// ensure bun version is at least 0.5.3, as that's when docker images started
		// getting published: https://hub.docker.com/r/oven/bun/tags
		out, err := exec.Command(bun, "-v").Output()
		if err != nil {
			return nil, nil
		} else {
			minVersion, err := semver.Make("0.5.3")
			if err != nil {
				panic(err)
			}

			bunVersionString := strings.TrimSpace(string(out))
			bunVersion, err := semver.Make(bunVersionString)

			if err != nil || bunVersion.LT(minVersion) {
				return nil, nil
			}

			srcInfo.Runtime = plan.RuntimeStruct{Language: "bun", Version: bunVersionString}
		}

		// set family
		srcInfo.Family = "Bun"
	}

	// extract deps and devdeps
	deps, ok := packageJson["dependencies"].(map[string]interface{})
	if !ok || deps == nil {
		deps = make(map[string]interface{})
	}
	devdeps, ok := packageJson["devDependencies"].(map[string]interface{})
	if !ok || devdeps == nil {
		devdeps = make(map[string]interface{})
	}
	scripts, ok := packageJson["scripts"].(map[string]interface{})
	if !ok || scripts == nil {
		scripts = make(map[string]interface{})
	}

	// infer db from dependencies
	if deps["pg"] != nil {
		srcInfo.DatabaseDesired = DatabaseKindPostgres
	} else if deps["mysql"] != nil {
		srcInfo.DatabaseDesired = DatabaseKindMySQL
	} else if deps["sqlite"] != nil || deps["better-sqlite3"] != nil {
		srcInfo.DatabaseDesired = DatabaseKindSqlite
	}

	// infer redis from dependencies
	if deps["redis"] != nil {
		srcInfo.RedisDesired = true
	}

	// infer object storage (Tigris) from dependencies
	if deps["@aws-sdk/client-s3"] != nil {
		srcInfo.ObjectStorageDesired = true
	}

	// if prisma is used, provider is definative
	if checksPass(sourceDir+"/prisma", dirContains("*.prisma", "provider")) {
		if checksPass(sourceDir+"/prisma", dirContains("*.prisma", "postgresql")) {
			srcInfo.DatabaseDesired = DatabaseKindPostgres
		} else if checksPass(sourceDir+"/prisma", dirContains("*.prisma", "mysql")) {
			srcInfo.DatabaseDesired = DatabaseKindMySQL
		} else if checksPass(sourceDir+"/prisma", dirContains("*.prisma", "sqlite")) {
			srcInfo.DatabaseDesired = DatabaseKindSqlite
			srcInfo.ObjectStorageDesired = true
		}
	}

	// don't prompt for redis or db unless they are used
	if srcInfo.DatabaseDesired != DatabaseKindPostgres && srcInfo.DatabaseDesired != DatabaseKindMySQL && !srcInfo.RedisDesired {
		srcInfo.SkipDatabase = true
	}

	// default to port 3000
	srcInfo.Port = 3000

	// etract port from EXPOSE statement in dockerfile
	dockerfile, err := os.ReadFile("Dockerfile")
	if err == nil {
		m := portRegex.FindStringSubmatch(string(dockerfile))

		for i, name := range portRegex.SubexpNames() {
			if len(m) > 0 && name == "port" {
				portFromDockerfile, err := strconv.Atoi(m[i])
				if err == nil {
					srcInfo.Port = portFromDockerfile
					continue
				}
			}
		}
	}

	// While redundant and requires dual matenance, it has been a point of
	// confusion for many when the framework detected is listed as "NodeJS"
	// See flyapps/dockerfile-node for the actual framework detction.
	// Also change PlatformMap in core.go if this list ever changes.
	if (deps["astro"] != nil) && (deps["@astrojs/node"] != nil) {
		srcInfo.Family = "Astro with SSR"
		srcInfo.Port = 4321
	} else if deps["astro"] != nil {
		srcInfo.Family = "Astro Static Site"
		srcInfo.Port = 80
	} else if deps["@adonisjs/core"] != nil {
		srcInfo.Family = "AdonisJS"
	} else if deps["gatsby"] != nil {
		srcInfo.Family = "Gatsby"
		srcInfo.Port = 8080
	} else if startScript, ok := scripts["start"].(string); ok && strings.Contains(startScript, "meteor") {
		srcInfo.Family = "Meteor"
		srcInfo.Env = map[string]string{
			"PORT":     "3000",
			"ROOT_URL": "APP_URL",
		}
	} else if deps["@nestjs/core"] != nil {
		srcInfo.Family = "NestJS"
	} else if deps["next"] != nil {
		srcInfo.Family = "Next.js"
	} else if deps["nust"] != nil {
		srcInfo.Family = "Nust"
	} else if devdeps["nuxt"] != nil || deps["nuxt"] != nil {
		srcInfo.Family = "Nuxt"
	} else if checksPass(sourceDir, fileExists("shopify.app.toml")) {
		srcInfo.Family = "Shopify"
	} else if deps["remix"] != nil || deps["@remix-run/node"] != nil {
		srcInfo.Family = "Remix"
	} else if devdeps["@sveltejs/kit"] != nil {
		srcInfo.Family = "SvelteKit"
	} else if scripts["dev"] == "vite" {
		srcInfo.Family = "Vite"
		srcInfo.Port = 80
	}

	return srcInfo, nil
}

func JsFrameworkCallback(appName string, srcInfo *SourceInfo, plan *plan.LaunchPlan, flags []string) error {
	// create temporary fly.toml for merge purposes
	flyToml := "fly.toml"
	_, err := os.Stat(flyToml)
	if os.IsNotExist(err) {
		// create a fly.toml consisting only of an app name
		contents := fmt.Sprintf("app = \"%s\"\n", appName)
		err := os.WriteFile(flyToml, []byte(contents), 0644)
		if err != nil {
			log.Fatal(err)
		}

		// inform caller of the presence of this file
		srcInfo.MergeConfig = &MergeConfigStruct{
			Name:      flyToml,
			Temporary: true,
		}
	}

	// add litestream if object storage is present and database is sqlite3
	if plan.ObjectStorage.Provider() != nil && srcInfo.DatabaseDesired == DatabaseKindSqlite {
		flags = append(flags, "--litestream")
	}

	// run dockerfile-node if Dockerfile doesn't already exist, or there is a database to be set up
	_, err = os.Stat("Dockerfile")
	if errors.Is(err, fs.ErrNotExist) || srcInfo.DatabaseDesired == DatabaseKindSqlite || srcInfo.DatabaseDesired == DatabaseKindPostgres {
		var args []string

		// add --skip flag if Dockerfile already exists
		if err == nil {
			flags = append([]string{"--skip"}, flags...)
		}

		_, err = os.Stat("node_modules")
		if errors.Is(err, fs.ErrNotExist) {
			// no existing node_modules directory: run package directly
			args = []string{"npx", "--yes", "@flydotio/dockerfile@latest"}

			// add additional flags from launch command
			if len(flags) > 0 {
				args = append(args, flags...)
			}
		} else {
			// build command to install package using preferred package manager
			args = []string{"npm", "install", "@flydotio/dockerfile@latest", "--save-dev"}

			_, err = os.Stat("yarn.lock")
			if !errors.Is(err, fs.ErrNotExist) {
				args = []string{"yarn", "add", "@flydotio/dockerfile@latest", "--dev"}
			}

			_, err = os.Stat("pnpm-lock.yaml")
			if !errors.Is(err, fs.ErrNotExist) {

				_, err = os.Stat("pnpm-workspace.yaml")
				if errors.Is(err, fs.ErrNotExist) {
					args = []string{"pnpm", "add", "-D", "@flydotio/dockerfile@latest"}
				} else {
					args = []string{"pnpm", "add", "-w", "-D", "@flydotio/dockerfile@latest"}
				}
			}

			_, err = os.Stat("bun.lockb")
			if !errors.Is(err, fs.ErrNotExist) {
				args = []string{"bun", "add", "-d", "@flydotio/dockerfile@latest"}
			}
		}

		// check first to see if the package is already installed
		installed := false

		deps, ok := packageJson["dependencies"].(map[string]interface{})
		if ok && deps["@flydotio/dockerfile"] != nil {
			installed = true
		}

		deps, ok = packageJson["devDependencies"].(map[string]interface{})
		if ok && deps["@flydotio/dockerfile"] != nil {
			installed = true
		}

		// install/run command
		if !installed || args[0] == "npx" {
			fmt.Printf("installing: %s\n", strings.Join(args[:], " "))
			cmd := exec.Command(args[0], args[1:]...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 42 {
					// generator exited with code 42, which means existing
					// Dockerfile contains errors which will prevent deployment.
					srcInfo.SkipDeploy = true
					srcInfo.DeployDocs = "Correct the errors and run 'fly deploy' to deploy your app."
					fmt.Println()
				} else {
					return fmt.Errorf("failed to install @flydotio/dockerfile: %w", err)
				}
			}
		}

		// run the package if we haven't already
		if args[0] != "npx" {
			// determine whether we need bunx or npx
			var xcmd string

			if args[0] == "bun" {
				xcmd = "bunx"
			} else {
				xcmd = "npx"
			}

			// build a new command
			args = []string{"dockerfile"}

			// find npx/bunx in path
			xcmdpath, err := exec.LookPath(xcmd)
			if err != nil && !errors.Is(err, exec.ErrDot) {
				if xcmd != "bunx" {
					return fmt.Errorf("failure finding %s executable in PATH", xcmd)
				} else {
					// switch to "bun x" if bunx is not found.
					// see: https://github.com/oven-sh/bun/issues/2786
					xcmd = "bun"
					xcmdpath, err = exec.LookPath("bun")
					if err != nil && !errors.Is(err, exec.ErrDot) {
						return fmt.Errorf("failure finding %s executable in PATH", xcmd)
					}
					args = append([]string{"x"}, args...)
				}
			}

			// resolve to absolute path, see: https://tip.golang.org/doc/go1.19#os-exec-path
			xcmdpath, err = filepath.Abs(xcmdpath)
			if err != nil {
				return fmt.Errorf("failure finding %s executable in PATH", xcmd)
			}

			// add additional flags from launch command
			if len(flags) > 0 {
				args = append(args, flags...)
			}

			// execute (via npx, bunx, or bun x) the docker module
			cmd := exec.Command(xcmdpath, args...)
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err != nil {
				if exitError, ok := err.(*exec.ExitError); ok && exitError.ExitCode() == 42 {
					// generator exited with code 42, which means existing
					// Dockerfile contains errors which will prevent deployment.
					srcInfo.SkipDeploy = true
					srcInfo.DeployDocs = "Correct the errors and run 'fly deploy' to deploy your app.\n"
					fmt.Println()
				} else {
					return fmt.Errorf("failed to generate Dockerfile: %w", err)
				}
			}
		}
	}

	// read dockerfile
	dockerfile, err := os.ReadFile("Dockerfile")
	if err != nil {
		return err
	}

	// extract family
	family := "NodeJS"
	re := regexp.MustCompile(`(?m)^LABEL\s+fly_launch_runtime="(?P<family>.+?)"`)
	m := re.FindStringSubmatch(string(dockerfile))

	for i, name := range re.SubexpNames() {
		if len(m) > 0 && name == "family" {
			family = m[i]
		}
	}
	srcInfo.Family = family

	// provide some advice
	if srcInfo.DeployDocs == "" {
		srcInfo.DeployDocs = fmt.Sprintf(`
If you need custom packages installed, or have problems with your deployment
build, you may need to edit the Dockerfile for app-specific changes. If you
need help, please post on https://community.fly.io.

Now: run 'fly deploy' to deploy your %s app.
`, srcInfo.Family)
	}

	return nil
}
