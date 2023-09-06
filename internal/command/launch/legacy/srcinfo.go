package legacy

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"github.com/samber/lo"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flaps"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/command/postgres"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/prompt"
	"github.com/superfly/flyctl/internal/set"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/scanner"
)

func createSourceInfoFiles(ctx context.Context, srcInfo *scanner.SourceInfo, workingDir string) error {
	if srcInfo == nil {
		return nil
	}

	io := iostreams.FromContext(ctx)

	for _, f := range srcInfo.Files {
		path := filepath.Join(workingDir, f.Path)
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

// If secrets are requested by the launch scanner, ask the user to input them
func createSecrets(ctx context.Context, srcInfo *scanner.SourceInfo, appName string) error {
	if srcInfo == nil || len(srcInfo.Secrets) == 0 {
		return nil
	}

	var err error
	io := iostreams.FromContext(ctx)
	secrets := map[string]string{}

	for _, secret := range srcInfo.Secrets {
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
		_, err := apiClient.SetSecrets(ctx, appName, secrets)
		if err != nil {
			return err
		}
		fmt.Fprintf(io.Out, "Set secrets on %s: %s\n", appName, strings.Join(lo.Keys(secrets), ", "))
	}
	return nil
}

func createVolumes(ctx context.Context, srcInfo *scanner.SourceInfo, appName string, regionCode string) error {
	if srcInfo == nil || len(srcInfo.Volumes) == 0 {
		return nil
	}
	io := iostreams.FromContext(ctx)

	flapsClient, err := flaps.NewFromAppName(ctx, appName)
	if err != nil {
		return err
	}

	for _, vol := range srcInfo.Volumes {
		volume, err := flapsClient.CreateVolume(ctx, api.CreateVolumeRequest{
			Name:      vol.Source,
			Region:    regionCode,
			SizeGb:    api.Pointer(1),
			Encrypted: api.Pointer(true),
		})
		if err != nil {
			return err
		} else {
			fmt.Fprintf(io.Out, "Created a %dGB volume %s in the %s region\n", volume.SizeGb, volume.ID, regionCode)
		}
	}
	return nil
}

func createDatabases(ctx context.Context, srcInfo *scanner.SourceInfo, appName string, region *api.Region, org *api.Organization) (set.Set[string], error) {
	var options set.Set[string]

	if srcInfo == nil || srcInfo.SkipDatabase || flag.GetBool(ctx, "no-deploy") || flag.GetBool(ctx, "now") {
		return options, nil
	}

	client := client.FromContext(ctx).API()
	io := iostreams.FromContext(ctx)
	colorize := io.ColorScheme()

	confirmPg, err := prompt.Confirm(ctx, "Would you like to set up a Postgresql database now?")
	if confirmPg && err == nil {
		db_app_name := fmt.Sprintf("%s-db", appName)
		should_attach_db := false

		if apps, err := client.GetApps(ctx, nil); err == nil {
			for _, app := range apps {
				if app.Name == db_app_name {
					msg := fmt.Sprintf("We found an existing Postgresql database with the name %s. Would you like to attach it to your app?", app.Name)
					confirmAttachPg, err := prompt.Confirm(ctx, msg)

					if confirmAttachPg && err == nil {
						should_attach_db = true
					}

				}
			}
		}

		options.Set("postgresql")

		if should_attach_db {
			// If we try to attach to a PG cluster with the usual username
			// format, we'll get an error (since that username already exists)
			// by generating a new username with a sufficiently random number
			// (in this case, the nanon second that the database is being attached)
			current_time := time.Now().Nanosecond()
			db_user := fmt.Sprintf("%s-%d", db_app_name, current_time)

			err = postgres.AttachCluster(ctx, postgres.AttachParams{
				PgAppName: db_app_name,
				AppName:   appName,
				DbUser:    db_user,
			})

			if err != nil {
				msg := "Failed attaching %s to the Postgres cluster %s: %s.\nTry attaching manually with 'fly postgres attach --app %s %s'\n"
				fmt.Fprintf(io.Out, msg, appName, db_app_name, err, appName, db_app_name)
			} else {
				fmt.Fprintf(io.Out, "Postgres cluster %s is now attached to %s\n", db_app_name, appName)
			}
		} else {
			err := LaunchPostgres(ctx, appName, org, region)
			if err != nil {
				const msg = "Error creating Postgresql database. Be warned that this may affect deploys"
				fmt.Fprintln(io.Out, colorize.Red(msg))
			}
		}
	}

	confirmRedis, err := prompt.Confirm(ctx, "Would you like to set up an Upstash Redis database now?")
	if confirmRedis && err == nil {
		err := LaunchRedis(ctx, appName, org, region)
		if err != nil {
			const msg = "Error creating Redis database. Be warned that this may affect deploys"
			fmt.Fprintln(io.Out, colorize.Red(msg))

		}

		options.Set("redis")
	}

	// Run any initialization commands required for Postgres if it was installed
	if confirmPg && len(srcInfo.PostgresInitCommands) > 0 {
		for _, cmd := range srcInfo.PostgresInitCommands {
			if cmd.Condition {
				if err := execInitCommand(ctx, cmd); err != nil {
					return options, err
				}
			}
		}
	}
	return options, nil
}

func setAppconfigFromSrcinfo(ctx context.Context, srcInfo *scanner.SourceInfo, appConfig *appconfig.Config) error {
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

	if srcInfo.SwapSizeMB > 0 {
		appConfig.SwapSizeMB = &srcInfo.SwapSizeMB
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

func runCallback(ctx context.Context, appName string, srcInfo *scanner.SourceInfo, plan *plan.LaunchPlan) error {
	if srcInfo == nil || srcInfo.Callback == nil {
		return nil
	}

	err := srcInfo.Callback(appName, srcInfo, plan)

	if srcInfo.MergeConfig != nil {
		if err == nil {
			cfg, err := appconfig.LoadConfig(srcInfo.MergeConfig.Name)
			if err == nil {
				// In theory, any part of the configuration could be merged here
				// currently supported configuration updates/overrides:
				// * health checks
				// * processes
				// * release command
				// * swap size
				// * volume
				if srcInfo.Processes == nil {
					srcInfo.Processes = cfg.Processes
				}

				if srcInfo.ReleaseCmd == "" && cfg.Deploy != nil {
					srcInfo.ReleaseCmd = cfg.Deploy.ReleaseCommand
				}

				if len(srcInfo.Volumes) == 0 && len(cfg.Mounts) > 0 {
					srcInfo.Volumes = []scanner.Volume{cfg.Mounts[0]}
				}

				if cfg.SwapSizeMB != nil {
					srcInfo.SwapSizeMB = *cfg.SwapSizeMB
				}

				if cfg.HTTPService != nil && cfg.HTTPService.HTTPChecks != nil {
					srcInfo.HttpCheckPath = *cfg.HTTPService.HTTPChecks[0].HTTPPath
				}
			}
		}

		if srcInfo.MergeConfig.Temporary {
			_ = os.Remove(srcInfo.MergeConfig.Name)
		}
	}

	return err
}

func runInitCommands(ctx context.Context, srcInfo *scanner.SourceInfo) error {
	if srcInfo != nil && len(srcInfo.InitCommands) > 0 {
		for _, cmd := range srcInfo.InitCommands {
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
