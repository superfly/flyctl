package launch

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

// satisfyScannerBeforeDb performs operations that the scanner requests that must be done before databases are created
func (state *launchState) satisfyScannerBeforeDb(ctx context.Context) error {
	if err := state.scannerCreateFiles(ctx); err != nil {
		return err
	}
	if err := state.scannerCreateSecrets(ctx); err != nil {
		return err
	}
	if err := state.scannerCreateVolumes(ctx); err != nil {
		return err
	}
	return nil
}

// satisfyScannerBeforeDb performs operations that the scanner requests that must be done after databases are created
func (state *launchState) satisfyScannerAfterDb(ctx context.Context, dbOptions map[string]bool) error {
	if err := state.scannerRunCallback(ctx, dbOptions); err != nil {
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
		// If a secret should be a random default, just generate it without displaying
		// Otherwise, prompt to type it in
		if secret.Generate != nil {
			if val, err = secret.Generate(); err != nil {
				return fmt.Errorf("could not generate random string: %w", err)
			}
		} else if secret.Value != "" {
			val = secret.Value
		} else {
			prompt := fmt.Sprintf("Set secret %s:", secret.Key)
			surveyInput := &survey.Input{Message: prompt, Help: secret.Help}
			survey.AskOne(surveyInput, &val)
		}

		if val != "" {
			secrets[secret.Key] = val
		}
	}

	if len(secrets) > 0 {
		apiClient := client.FromContext(ctx).API()
		_, err := apiClient.SetSecrets(ctx, state.plan.AppName, secrets)
		if err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Set secrets on %s: %s\n", state.plan.AppName, strings.Join(lo.Keys(secrets), ", "))
	}
	return nil
}

func (state *launchState) scannerCreateVolumes(ctx context.Context) error {
	if state.sourceInfo == nil || len(state.sourceInfo.Volumes) == 0 {
		return nil
	}
	io := iostreams.FromContext(ctx)
	client := client.FromContext(ctx).API()

	for _, vol := range state.sourceInfo.Volumes {
		appID, err := client.GetAppID(ctx, state.plan.AppName)
		if err != nil {
			return err
		}

		volume, err := client.CreateVolume(ctx, api.CreateVolumeInput{
			AppID:     appID,
			Name:      vol.Source,
			Region:    state.plan.RegionCode,
			SizeGb:    1,
			Encrypted: true,
		})
		if err != nil {
			return err
		} else {
			fmt.Fprintf(io.Out, "Created a %dGB volume %s in the %s region\n", volume.SizeGb, volume.ID, state.plan.RegionCode)
		}
	}
	return nil
}

func (state *launchState) scannerRunCallback(ctx context.Context, dbOptions map[string]bool) error {
	if state.sourceInfo == nil || state.sourceInfo.Callback == nil {
		return nil
	}

	err := state.sourceInfo.Callback(state.plan.AppName, state.sourceInfo, dbOptions)

	if state.sourceInfo.MergeConfig != nil {
		if err == nil {
			cfg, err := appconfig.LoadConfig(state.sourceInfo.MergeConfig.Name)
			if err == nil {
				// In theory, any part of the configuration could be merged here, but for now
				// we will only copy over the processes, release command, and volume
				if state.sourceInfo.Processes == nil {
					state.sourceInfo.Processes = cfg.Processes
				}

				if state.sourceInfo.ReleaseCmd == "" && cfg.Deploy != nil {
					state.sourceInfo.ReleaseCmd = cfg.Deploy.ReleaseCommand
				}

				if len(state.sourceInfo.Volumes) == 0 && len(cfg.Mounts) > 0 {
					state.sourceInfo.Volumes = []scanner.Volume{cfg.Mounts[0]}
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

	if srcInfo.Port > 0 {
		appConfig.SetInternalPort(srcInfo.Port)
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
				Source:      v.Source,
				Destination: v.Destination,
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
