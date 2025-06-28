package launch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/samber/lo"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/oci"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
	"github.com/superfly/flyctl/terminal"
)

func (state *launchState) setupGitHubActions(ctx context.Context, appName string) error {
	if flag.GetBool(ctx, "no-github-workflow") || flag.GetString(ctx, "from") != "" {
		return nil
	}

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
			if !flag.GetBool(ctx, "yes") {
				confirm, err := prompt.ConfirmOverwrite(ctx, path)
				if !confirm || err != nil {
					continue
				}
			} else {
				fmt.Fprintf(io.Out, "You specified --yes, overwriting %s\n", path)
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

	// Handle multi-container configurations from Docker Compose
	if len(srcInfo.Containers) > 0 {
		return state.scannerSetMultiContainerConfig(ctx)
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

// scannerSetMultiContainerConfig handles multi-container configurations from Docker Compose
func (state *launchState) scannerSetMultiContainerConfig(ctx context.Context) error {
	srcInfo := state.sourceInfo
	appConfig := state.appConfig

	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	// For multi-container apps, we need to use Pilot as the init system
	fmt.Fprintf(io.Out, "%s Detected multi-container application with %d services\n",
		colorize.SuccessIcon(), len(srcInfo.Containers))

	// Set up basic app configuration
	if srcInfo.Port != 0 {
		if appConfig.HTTPService == nil {
			appConfig.HTTPService = &appconfig.HTTPService{
				InternalPort:       srcInfo.Port,
				ForceHTTPS:         true,
				AutoStartMachines:  fly.Pointer(true),
				AutoStopMachines:   fly.Pointer(fly.MachineAutostopStop),
				MinMachinesRunning: fly.Pointer(0),
			}
		} else {
			appConfig.HTTPService.InternalPort = srcInfo.Port
		}
	}

	// Apply global environment variables
	for envName, envVal := range srcInfo.Env {
		if envVal == "APP_FQDN" {
			appConfig.SetEnvVariable(envName, appConfig.AppName+".fly.dev")
		} else if envVal == "APP_URL" {
			appConfig.SetEnvVariable(envName, "https://"+appConfig.AppName+".fly.dev")
		} else {
			appConfig.SetEnvVariable(envName, envVal)
		}
	}

	// Set up volumes
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

	// Write the entrypoint script to a file
	entrypointScript := getEntrypointScript(srcInfo.Files)
	entrypointPath := "fly-entrypoint.sh"
	if err := os.WriteFile(entrypointPath, entrypointScript, 0644); err != nil {
		return fmt.Errorf("failed to write entrypoint script: %w", err)
	}
	// Explicitly set executable permissions
	if err := os.Chmod(entrypointPath, 0755); err != nil {
		return fmt.Errorf("failed to set entrypoint script permissions: %w", err)
	}

	fmt.Fprintf(io.Out, "  Created entrypoint script: %s\n", entrypointPath)

	// Generate machine configuration for multi-container setup
	machineConfig := state.generateMultiContainerMachineConfig()

	// Write machine configuration to a separate file
	machineConfigPath := "fly.machine.json"
	machineConfigData, err := json.MarshalIndent(machineConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal machine configuration: %w", err)
	}

	if err := os.WriteFile(machineConfigPath, machineConfigData, 0644); err != nil {
		return fmt.Errorf("failed to write machine configuration: %w", err)
	}

	// Reference the machine config file in fly.toml
	appConfig.MachineConfig = machineConfigPath

	// Set the container field if specified (for single build service)
	if srcInfo.Container != "" {
		appConfig.Container = srcInfo.Container
	}

	fmt.Fprintf(io.Out, "  Created machine configuration file: %s\n", machineConfigPath)
	fmt.Fprintf(io.Out, "  Note: All containers will run in the same VM with shared networking\n")

	// Add database notices if needed
	if srcInfo.DatabaseDesired != scanner.DatabaseKindNone {
		fmt.Fprintf(io.Out, "\n%s Database service detected in docker-compose.yml\n", colorize.Yellow("!"))
		fmt.Fprintf(io.Out, "  Consider using Fly.io managed databases for better performance and reliability\n")
	}

	if srcInfo.RedisDesired {
		fmt.Fprintf(io.Out, "\n%s Redis service detected in docker-compose.yml\n", colorize.Yellow("!"))
		fmt.Fprintf(io.Out, "  Consider using Fly.io Upstash Redis for better performance and reliability\n")
	}

	return nil
}

// generateMultiContainerMachineConfig creates the machine configuration for multi-container apps
func (state *launchState) generateMultiContainerMachineConfig() map[string]interface{} {
	srcInfo := state.sourceInfo

	// Create containers array for the machine configuration
	containers := []map[string]interface{}{}

	for _, container := range srcInfo.Containers {
		containerConfig := map[string]interface{}{
			"name": container.Name,
		}

		// Check if this is the container with the build section that should be omitted
		if srcInfo.Container != "" && container.Name == srcInfo.Container {
			// For the service with build section, omit both build and image fields
			// The container will be built from the fly.toml build configuration
		} else {
			// Set image or build configuration for other containers
			if container.Image != "" {
				containerConfig["image"] = container.Image
			} else if container.BuildContext != "" {
				// For build contexts, we'll need to handle this during deployment
				// For now, mark it as needing build
				containerConfig["image"] = fmt.Sprintf("registry.fly.io/%s:%s", state.appConfig.AppName, container.Name)
				containerConfig["build"] = map[string]interface{}{
					"context":    container.BuildContext,
					"dockerfile": container.Dockerfile,
				}
			}
		}

		// Set command and entrypoint
		if container.UseImageDefaults {
			// For containers that need image defaults, don't override entrypoint or cmd
			// Let the image use its default ENTRYPOINT and CMD
			terminal.Debugf("Container %s using image defaults, no entrypoint/cmd override", container.Name)
		} else if len(container.Entrypoint) > 0 {
			containerConfig["entrypoint"] = container.Entrypoint
			// When entrypoint is set, include "cmd" if we have one
			if len(container.Command) > 0 {
				containerConfig["cmd"] = container.Command
			}
		} else if len(container.Command) > 0 {
			// No custom entrypoint, use standard command
			containerConfig["command"] = container.Command
		}

		// Set environment variables
		if len(container.Env) > 0 {
			containerConfig["environment"] = container.Env
		}

		// Set dependencies
		if len(container.DependsOn) > 0 {
			dependencies := []map[string]interface{}{}
			for _, dep := range container.DependsOn {
				dependencies = append(dependencies, map[string]interface{}{
					"container": dep.Name,
					"condition": dep.Condition,
				})
			}
			containerConfig["depends_on"] = dependencies
		}

		// Set health check
		if container.HealthCheck != nil {
			healthcheck := map[string]interface{}{
				"test": container.HealthCheck.Test,
			}
			if container.HealthCheck.Interval != "" {
				healthcheck["interval"] = container.HealthCheck.Interval
			}
			if container.HealthCheck.Timeout != "" {
				healthcheck["timeout"] = container.HealthCheck.Timeout
			}
			if container.HealthCheck.StartPeriod != "" {
				healthcheck["start_period"] = container.HealthCheck.StartPeriod
			}
			if container.HealthCheck.Retries > 0 {
				healthcheck["retries"] = container.HealthCheck.Retries
			}
			containerConfig["healthcheck"] = healthcheck
		}

		// Set restart policy
		if container.RestartPolicy != "" {
			containerConfig["restart"] = container.RestartPolicy
		}

		// Set secrets that this container needs access to
		if len(container.Secrets) > 0 {
			secrets := []map[string]interface{}{}
			for _, secretName := range container.Secrets {
				// Create secret config with just env_var
				// The "name" field is omitted when it's the same as env_var
				secretConfig := map[string]interface{}{
					"env_var": secretName,
				}
				secrets = append(secrets, secretConfig)
			}
			containerConfig["secrets"] = secrets
		}

		containers = append(containers, containerConfig)
	}

	// Add entrypoint script file reference for service discovery only to containers that use it
	for i, containerMap := range containers {
		// Check if this container uses the service discovery entrypoint
		if entrypoint, exists := containerMap["entrypoint"]; exists {
			if entrypointSlice, ok := entrypoint.([]string); ok && len(entrypointSlice) > 0 && entrypointSlice[0] == "/fly-entrypoint.sh" {
				containers[i]["files"] = []map[string]interface{}{
					{
						"guest_path": "/fly-entrypoint.sh",
						"local_path": "fly-entrypoint.sh",
						"mode":       0755,
					},
				}
			}
		}
	}

	// Create the machine configuration for multi-container deployment
	// Pilot is used automatically for multi-container machines
	machineConfig := map[string]interface{}{
		"containers": containers,
	}

	return machineConfig
}

// getEntrypointScript extracts the entrypoint script from SourceInfo files
func getEntrypointScript(files []scanner.SourceFile) []byte {
	for _, file := range files {
		if file.Path == "/fly-entrypoint.sh" {
			return file.Contents
		}
	}
	// Fallback script if not found (shouldn't happen)
	return []byte(`#!/bin/sh
set -e
echo "127.0.0.1 localhost" >> /etc/hosts
exec "$@"
`)
}

// extractCmdFromImage extracts the CMD from a Docker image using OCI inspection
func (state *launchState) extractCmdFromImage(imageName string) []string {
	if imageName == "" {
		return nil
	}

	imageConfig, err := oci.GetImageConfig(imageName, nil)
	if err != nil {
		terminal.Debugf("Warning: failed to extract CMD from image %s: %v", imageName, err)
		return nil
	}

	if len(imageConfig.Cmd) > 0 {
		terminal.Debugf("Extracted CMD from image %s: %v", imageName, imageConfig.Cmd)
		return imageConfig.Cmd
	}

	terminal.Debugf("No CMD found in image %s", imageName)
	return nil
}
