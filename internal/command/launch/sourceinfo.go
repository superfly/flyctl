package launch

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/cavaliergopher/grab/v3"
	"github.com/logrusorgru/aurora"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

func determineSourceInfo(ctx context.Context, appConfig *appconfig.Config, copyConfig bool, workingDir string) (*scanner.SourceInfo, *appconfig.Build, error) {
	io := iostreams.FromContext(ctx)
	build := &appconfig.Build{}
	srcInfo := &scanner.SourceInfo{}
	var err error

	scannerConfig := &scanner.ScannerConfig{
		ExistingPort: appConfig.InternalPort(),
		Mode:         "launch",
		Colorize:     io.ColorScheme(),
	}
	// Detect if --copy-config and --now flags are set. If so, limited set of
	// fly.toml file updates. Helpful for deploying PRs when the project is
	// already setup and we only need fly.toml config changes.
	if copyConfig && flag.GetBool(ctx, "now") {
		scannerConfig.Mode = "clone"
	}

	if img := flag.GetString(ctx, "image"); img != "" {
		fmt.Fprintln(io.Out, "Using image", img)
		build.Image = img

		return srcInfo, build, nil
	}

	if dockerfile := flag.GetString(ctx, "dockerfile"); dockerfile != "" {
		if strings.HasPrefix(dockerfile, "http://") || strings.HasPrefix(dockerfile, "https://") {
			fmt.Fprintln(io.Out, "Downloading dockerfile", dockerfile)
			resp, err := grab.Get("Dockerfile", dockerfile)
			if err != nil {
				return nil, nil, err
			}
			dockerfile = resp.Filename
		}
		fmt.Fprintln(io.Out, "Using dockerfile", dockerfile)
		build.Dockerfile = dockerfile

		srcInfo, err = scanner.ScanDockerfile(dockerfile, scannerConfig)
		if err != nil {
			return nil, nil, err
		}

		return srcInfo, build, nil
	}

	if strategies := appConfig.BuildStrategies(); len(strategies) > 0 {
		fmt.Fprintf(io.Out, "Using build strategies '%s'. Remove [build] from fly.toml to force a rescan\n", aurora.Yellow(strategies))

		return srcInfo, appConfig.Build, nil
	}

	planStep := plan.GetPlanStep(ctx)

	if planStep == "" || planStep == "generate" {
		fmt.Fprintln(io.Out, "Scanning source code")
	}

	srcInfo, err = scanner.Scan(workingDir, scannerConfig)
	if err != nil {
		return nil, nil, err
	}
	if srcInfo != nil && srcInfo.DockerfilePath != "" {
		dockerfilePath, err := filepath.Rel(workingDir, srcInfo.DockerfilePath)
		if err != nil {
			return nil, nil, fmt.Errorf("could not resolve Dockerfile path: %w", err)
		}
		if dockerfilePath != "Dockerfile" {
			build.Dockerfile = dockerfilePath
		}
	}

	if srcInfo == nil {
		msg := "Could not find a Dockerfile, nor detect a runtime or framework from source code."
		var noBlankErr error
		switch {
		case planStep == "propose":
			noBlankErr = errors.New("Could not detect runtime or Dockerfile")
		case (!io.IsInteractive() || env.IsCI()) && (flag.GetBool(ctx, "now") || !flag.GetBool(ctx, "no-deploy")) && !flag.GetBool(ctx, "manifest") && !noCreateApp(ctx):
			// A blank app is never deployed. Without a terminal nobody sees
			// that, and the empty app plus exit 0 would pass for a launch.
			// --manifest only prints the plan and --no-create-app only writes
			// fly.toml, so there's nothing to mistake. --now overrides
			// --no-deploy, as in firstDeploy.
			noDeploy := "pass --no-deploy"
			if flag.GetBool(ctx, "now") {
				noDeploy = "drop --now and pass --no-deploy"
			}
			noBlankErr = fmt.Errorf("found nothing to build; add a Dockerfile, or %s to create an empty app", noDeploy)
		}
		if noBlankErr != nil {
			fmt.Fprintln(io.Out, aurora.Red(msg))

			return nil, nil, noBlankErr
		}
		fmt.Fprintln(io.Out, aurora.Green(msg+" Continuing with a blank app."))

		return srcInfo, nil, err
	}

	appType := srcInfo.Family
	if srcInfo.Version != "" {
		appType = appType + " " + srcInfo.Version
	}

	if planStep == "" || planStep == "generate" {
		fmt.Fprintf(io.Out, "Detected %s %s app\n", articleFor(srcInfo.Family), aurora.Green(appType))
	}

	if srcInfo.Builder != "" {
		fmt.Fprintln(io.Out, "Using the following build configuration:")
		fmt.Fprintln(io.Out, "\tBuilder:", srcInfo.Builder)
		if len(srcInfo.Buildpacks) > 0 {
			fmt.Fprintln(io.Out, "\tBuildpacks:", strings.Join(srcInfo.Buildpacks, " "))
		}

		build = &appconfig.Build{
			Builder:    srcInfo.Builder,
			Buildpacks: srcInfo.Buildpacks,
		}
	}

	return srcInfo, build, nil
}

func articleFor(w string) string {
	var article = "a"
	if matched, _ := regexp.MatchString(`^[aeiou]`, strings.ToLower(w)); matched {
		article += "n"
	}

	return article
}

// noCreateApp reports whether launch should leave app creation alone.
// --no-create-app wins when given, as in ApplyAliases; otherwise read
// --no-create, its alias or plan propose's own flag.
func noCreateApp(ctx context.Context) bool {
	if flag.IsSpecified(ctx, "no-create-app") {
		return flag.GetBool(ctx, "no-create-app")
	}
	return flag.GetBool(ctx, "no-create")
}
