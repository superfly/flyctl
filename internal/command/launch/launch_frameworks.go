package launch

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

func (state *launchState) setupGitHubActions(ctx context.Context, appName string) error {
	state.sourceInfo.Files = append(state.sourceInfo.Files, state.sourceInfo.GitHubActions.Files...)

	if state.sourceInfo.GitHubActions.Secrets {
		gh, err := exec.LookPath("gh")

		if err != nil {
			fmt.Println("Run `fly tokens create deploy -x 999999h` to create a token and set it as the FLY_API_TOKEN secret in your GitHub repository settings")
			fmt.Println("See https://docs.github.com/en/actions/security-guides/using-secrets-in-github-actions")
		} else {
			apiClient := flyutil.ClientFromContext(ctx)

			expiry := "999999h"

			app, err := apiClient.GetAppCompact(ctx, appName)
			if err != nil {
				return fmt.Errorf("failed retrieving app %s: %w", appName, err)
			}

			resp, err := gql.CreateLimitedAccessToken(
				ctx,
				apiClient.GenqClient(),
				appName,
				app.Organization.ID,
				"deploy",
				&gql.LimitedAccessTokenOptions{
					"app_id": app.ID,
				},
				expiry,
			)
			if err != nil {
				return fmt.Errorf("failed creating token: %w", err)
			} else {
				token := resp.CreateLimitedAccessToken.LimitedAccessToken.TokenHeader

				fmt.Println("Setting FLY_API_TOKEN secret in GitHub repository settings")
				cmd := exec.Command(gh, "secret", "set", "FLY_API_TOKEN")
				cmd.Stdin = strings.NewReader(token)
				err = cmd.Run()

				if err != nil {
					fmt.Println("failed setting FLY_API_TOKEN secret in GitHub repository settings: %w", err)
				}
			}
		}
	}

	return nil
}

// satisfyScannerBeforeDb performs operations that the scanner requests that must be done before databases are created
func (state *launchState) satisfyScannerBeforeDb(ctx context.Context) error {
	if err := state.scannerCreateFiles(ctx); err != nil {
		return err
	}
	if err := state.scannerCreateSecrets(ctx); err != nil {
		return err
	}
	return nil
}

// satisfyScannerBeforeDb performs operations that the scanner requests that must be done after databases are created
func (state *launchState) satisfyScannerAfterDb(ctx context.Context) error {
	if err := state.scannerRunCallback(ctx); err != nil {
		return err
	}
	if err := state.scannerRunInitCommands(ctx); err != nil {
		return err
	}
	if err := state.scannerSetAppconfig(ctx); err != nil {
		return err
	}
	return nil
}

func (state *launchState) scannerCreateFiles(ctx context.Context) error {
	if state.sourceInfo == nil {
		return nil
	}

	io := iostreams.FromContext(ctx)

	for _, f := range state.sourceInfo.Files {
		path := filepath.Join(state.workingDir, f.Path)
		if helpers.FileExists(path) {
			if flag.GetBool(ctx, "now") {
				fmt.Fprintf(io.Out, "You specified --now, so not overwriting %s\n", path)
				continue
			}
			confirm, err := prompt.ConfirmOverwrite(ctx, path)
			if !confirm || err != nil {
				continue
			}
		}

		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}

		perms := 0o600
		if strings.Contains(string(f.Contents), "#!") {
			perms = 0o700
		}

		if err := os.WriteFile(path, f.Contents, fs.FileMode(perms)); err != nil {
			return err
		}
	}
	return nil
}

func (state *launchState) scannerCreateSecrets(ctx context.Context) error {
	if state.sourceInfo == nil || len(state.sourceInfo.Secrets) == 0 {
		return nil
	}

	var err error
	io := iostreams.FromContext(ctx)
	secrets := map[string]string{}

	for _, secret := range state.sourceInfo.Secrets {
		val := ""

		switch {
		case secret.Generate != nil:
			if val, err = secret.Generate(); err != nil {
				return fmt.Errorf("could not generate random string: %w", err)
			}
		case secret.Value != "":
			val = secret.Value
		default:
			message := fmt.Sprintf("Set secret %s:", secret.Key)
			err = prompt.StringWithHelp(ctx, &val, message, "", secret.Help, false)
			if err != nil && !errors.Is(err, prompt.ErrNonInteractive) {
				return err
			}
		}
		if val != "" {
			secrets[secret.Key] = val
		}
	}

	if len(secrets) > 0 {
		apiClient := flyutil.ClientFromContext(ctx)
		_, err := apiClient.SetSecrets(ctx, state.Plan.AppName, secrets)
		if err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Set secrets on %s: %s\n", state.Plan.AppName, strings.Join(lo.Keys(secrets), ", "))
	}
	return nil
}

func (state *launchState) scannerRunCallback(ctx context.Context) error {
	if state.sourceInfo == nil || state.sourceInfo.Callback == nil {
		return nil
	}

	err := state.sourceInfo.Callback(state.Plan.AppName, state.sourceInfo, state.Plan, flag.ExtraArgsFromContext(ctx))

	if state.sourceInfo.MergeConfig != nil {
		if err == nil {
			cfg, err := appconfig.LoadConfig(state.sourceInfo.MergeConfig.Name)
			if err == nil {
				// In theory, any part of the configuration could be merged here, but for now
				// we will only copy over the processes, release command, env, volume, and statics
				if state.sourceInfo.Processes == nil {
					state.sourceInfo.Processes = cfg.Processes
				}

				if state.sourceInfo.ReleaseCmd == "" && cfg.Deploy != nil {
					state.sourceInfo.ReleaseCmd = cfg.Deploy.ReleaseCommand
				}

				if state.sourceInfo.SeedCmd == "" && cfg.Deploy != nil {
					state.sourceInfo.SeedCmd = cfg.Deploy.SeedCommand
				}

				if len(cfg.Env) > 0 {
					if len(state.sourceInfo.Env) == 0 {
						state.sourceInfo.Env = cfg.Env
					} else {
						clone := maps.Clone(state.sourceInfo.Env)
						maps.Copy(clone, cfg.Env)
						state.sourceInfo.Env = clone
					}
				}

				if len(state.sourceInfo.Volumes) == 0 && len(cfg.Mounts) > 0 {
					state.sourceInfo.Volumes = []scanner.Volume{cfg.Mounts[0]}
				}

				if len(state.sourceInfo.Statics) == 0 && len(cfg.Statics) > 0 {
					state.sourceInfo.Statics = cfg.Statics
				}
			}
		}

		if state.sourceInfo.MergeConfig.Temporary {
			_ = os.Remove(state.sourceInfo.MergeConfig.Name)
		}
	}

	return err
}

func (state *launchState) scannerRunInitCommands(ctx context.Context) error {
	if state.sourceInfo != nil && len(state.sourceInfo.InitCommands) > 0 {
		for _, cmd := range state.sourceInfo.InitCommands {
			if err := execInitCommand(ctx, cmd); err != nil {
				return err
			}
		}
	}

	if state.sourceInfo != nil && state.sourceInfo.PostInitCallback != nil {
		if err := state.sourceInfo.PostInitCallback(); err != nil {
			return err
		}
	}

	return nil
}

func execInitCommand(ctx context.Context, command scanner.InitCommand) (err error) {
	io := iostreams.FromContext(ctx)

	binary, err := exec.LookPath(command.Command)
	if err != nil {
		return fmt.Errorf("%s not found in $PATH - make sure app dependencies are installed and try again", command.Command)
	}
	fmt.Fprintln(io.Out, command.Description)
	// Run a requested generator command, for example to generate a Dockerfile
	cmd := exec.CommandContext(ctx, binary, command.Args...)

	if err = cmd.Start(); err != nil {
		return err
	}

	if err = cmd.Wait(); err != nil {
		err = fmt.Errorf("failed running %s: %w ", cmd.String(), err)
	}
	return err
}

func (state *launchState) scannerSetAppconfig(ctx context.Context) error {
	srcInfo := state.sourceInfo
	appConfig := state.appConfig

	// Complete the appConfig
	if srcInfo == nil {
		return nil
	}

	if srcInfo.HttpCheckPath != "" {
		appConfig.SetHttpCheck(srcInfo.HttpCheckPath, srcInfo.HttpCheckHeaders)
	}

	if srcInfo.Concurrency != nil {
		appConfig.SetConcurrency(srcInfo.Concurrency["soft_limit"], srcInfo.Concurrency["hard_limit"])
	}

	for envName, envVal := range srcInfo.Env {
		if envVal == "APP_FQDN" {
			appConfig.SetEnvVariable(envName, appConfig.AppName+".fly.dev")
		} else if envVal == "APP_URL" {
			appConfig.SetEnvVariable(envName, "https://"+appConfig.AppName+".fly.dev")
		} else {
			appConfig.SetEnvVariable(envName, envVal)
		}
	}

	if len(srcInfo.Statics) > 0 {
		var appStatics []appconfig.Static
		for _, s := range srcInfo.Statics {
			appStatics = append(appStatics, appconfig.Static{
				GuestPath: s.GuestPath,
				UrlPrefix: s.UrlPrefix,
			})
		}
		appConfig.SetStatics(appStatics)
	}

	if len(srcInfo.Volumes) > 0 {
		var appVolumes []appconfig.Mount
		for _, v := range srcInfo.Volumes {
			appVolumes = append(appVolumes, appconfig.Mount{
				Source:                  v.Source,
				Destination:             v.Destination,
				AutoExtendSizeThreshold: v.AutoExtendSizeThreshold,
				AutoExtendSizeIncrement: v.AutoExtendSizeIncrement,
				AutoExtendSizeLimit:     v.AutoExtendSizeLimit,
			})
		}
		appConfig.SetMounts(appVolumes)
	}

	if len(srcInfo.Processes) > 0 {
		for procName, procCommand := range srcInfo.Processes {
			appConfig.SetProcess(procName, procCommand)

			// if processes are defined, associate HTTPService with the app service
			// (if defined) or the first service if no app service is defined.
			if appConfig.HTTPService != nil &&
				(procName == "app" || appConfig.HTTPService.Processes == nil) {
				appConfig.HTTPService.Processes = []string{procName}
			}
		}
	}

	if srcInfo.ReleaseCmd != "" {
		appConfig.SetReleaseCommand(srcInfo.ReleaseCmd)
	}

	if srcInfo.SeedCmd != "" {
		// no V1 compatibility for this feature so bypass setters
		appConfig.Deploy.SeedCommand = srcInfo.SeedCmd
	}

	if srcInfo.DockerCommand != "" {
		appConfig.SetDockerCommand(srcInfo.DockerCommand)
	}

	if srcInfo.ConsoleCommand != "" {
		// no V1 compatibility for this feature so bypass setters
		appConfig.ConsoleCommand = srcInfo.ConsoleCommand
	}

	if srcInfo.DockerEntrypoint != "" {
		appConfig.SetDockerEntrypoint(srcInfo.DockerEntrypoint)
	}

	if srcInfo.KillSignal != "" {
		appConfig.SetKillSignal(srcInfo.KillSignal)
	}

	// Append any requested Dockerfile entries
	if len(srcInfo.DockerfileAppendix) > 0 {
		if err := appendDockerfileAppendix(srcInfo.DockerfileAppendix); err != nil {
			return fmt.Errorf("failed appending Dockerfile appendix: %w", err)
		}
	}

	if len(srcInfo.BuildArgs) > 0 {
		if appConfig.Build == nil {
			appConfig.Build = &appconfig.Build{}
		}
		appConfig.Build.Args = srcInfo.BuildArgs
	}
	return nil
}
