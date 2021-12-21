package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/dustin/go-humanize"
	"github.com/logrusorgru/aurora"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/cmdctx"
	"github.com/superfly/flyctl/docstrings"
	"github.com/superfly/flyctl/internal/build/imgsrc"
	"github.com/superfly/flyctl/internal/client"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/cmdutil"
	"github.com/superfly/flyctl/pkg/logs"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"
)

func newDeployCommand(client *client.Client) *Command {
	deployStrings := docstrings.Get("deploy")
	cmd := BuildCommandKS(nil, runDeploy, deployStrings, client, workingDirectoryFromArg(0), requireSession, requireAppName)
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "image",
		Shorthand:   "i",
		Description: "Image tag or id to deploy",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "detach",
		Description: "Return immediately instead of monitoring deployment progress",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name: "build-only",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "remote-only",
		Description: "Perform builds remotely without using the local docker daemon",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "local-only",
		Description: "Only perform builds locally using the local docker daemon",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "strategy",
		Description: "The strategy for replacing running instances. Options are canary, rolling, bluegreen, or immediate. Default is canary, or rolling when max-per-region is set.",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "dockerfile",
		Description: "Path to a Dockerfile. Defaults to the Dockerfile in the working directory.",
	})
	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "build-arg",
		Description: "Set of build time variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	})
	cmd.AddStringSliceFlag(StringSliceFlagOpts{
		Name:        "env",
		Shorthand:   "e",
		Description: "Set of environment variables in the form of NAME=VALUE pairs. Can be specified multiple times.",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "image-label",
		Description: "Image label to use when tagging and pushing to the fly registry. Defaults to \"deployment-{timestamp}\".",
	})
	cmd.AddStringFlag(StringFlagOpts{
		Name:        "build-target",
		Description: "Set the target build stage to build if the Dockerfile has more than one stage",
	})
	cmd.AddBoolFlag(BoolFlagOpts{
		Name:        "no-cache",
		Description: "Do not use the cache when building the image",
	})

	cmd.Command.Args = cobra.MaximumNArgs(1)

	return cmd
}

func runDeploy(cmdCtx *cmdctx.CmdContext) error {
	ctx := cmdCtx.Command.Context()

	cmdCtx.Status("deploy", cmdctx.STITLE, "Deploying", cmdCtx.AppName)

	cmdfmt.PrintBegin(cmdCtx.Out, "Validating app configuration")

	if cmdCtx.AppConfig == nil {
		cfg, err := cmdCtx.Client.API().GetConfig(ctx, cmdCtx.AppName)
		if err != nil {
			return fmt.Errorf("unable to fetch existing configuration file: %s", err)
		}

		cmdCtx.AppConfig.Definition = cfg.Definition
	}

	if extraEnv := cmdCtx.Config.GetStringSlice("env"); len(extraEnv) > 0 {
		parsedEnv, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("env"))
		if err != nil {
			return errors.Wrap(err, "invalid env")
		}
		cmdCtx.AppConfig.SetEnvVariables(parsedEnv)
	}

	parsedCfg, err := cmdCtx.Client.API().ParseConfig(ctx, cmdCtx.AppName, cmdCtx.AppConfig.Definition)
	if err != nil {
		if parsedCfg == nil {
			// No error data has been returned
			return fmt.Errorf("not possible to validate configuration: server returned %s", err)
		}
		for _, error := range parsedCfg.Errors {
			//	fmt.Println("   ", aurora.Red("✘").String(), error)
			cmdCtx.Status("deploy", cmdctx.SERROR, "   ", aurora.Red("✘").String(), error)
		}
		return err
	}
	cmdCtx.AppConfig.Definition = parsedCfg.Definition
	cmdfmt.PrintDone(cmdCtx.Out, "Validating app configuration done")

	if parsedCfg.Valid && len(parsedCfg.Services) > 0 {
		cmdfmt.PrintServicesList(cmdCtx.IO, parsedCfg.Services)
	}

	daemonType := imgsrc.NewDockerDaemonType(!cmdCtx.Config.GetBool("remote-only"), !cmdCtx.Config.GetBool("local-only"))
	resolver := imgsrc.NewResolver(daemonType, cmdCtx.Client.API(), cmdCtx.AppName, cmdCtx.IO)

	var img *imgsrc.DeploymentImage

	var imageRef string
	if ref := cmdCtx.Config.GetString("image"); ref != "" {
		imageRef = ref
	} else if ref := cmdCtx.AppConfig.Image(); ref != "" {
		imageRef = ref
	}

	if imageRef != "" {
		opts := imgsrc.RefOptions{
			AppName:    cmdCtx.AppName,
			WorkingDir: cmdCtx.WorkingDir,
			Publish:    !cmdCtx.Config.GetBool("build-only"),
			ImageRef:   imageRef,
			ImageLabel: cmdCtx.Config.GetString("image-label"),
		}

		img, err = resolver.ResolveReference(ctx, cmdCtx.IO, opts)
		if err != nil {
			return err
		}
	} else {
		opts := imgsrc.ImageOptions{
			AppName:    cmdCtx.AppName,
			WorkingDir: cmdCtx.WorkingDir,
			Publish:    !cmdCtx.Config.GetBool("build-only"),
			ImageLabel: cmdCtx.Config.GetString("image-label"),
			NoCache:    cmdCtx.Config.GetBool("no-cache"),
		}

		if dockerfilePath := cmdCtx.Config.GetString("dockerfile"); dockerfilePath != "" {
			dockerfilePath, err := filepath.Abs(dockerfilePath)
			if err != nil {
				return err
			}
			opts.DockerfilePath = dockerfilePath
		} else if dockerfilePath := cmdCtx.AppConfig.Dockerfile(); dockerfilePath != "" {
			opts.DockerfilePath = filepath.Join(filepath.Dir(cmdCtx.ConfigFile), dockerfilePath)
		}

		if dockerBuildTarget := cmdCtx.Config.GetString("build-target"); dockerBuildTarget != "" {
			opts.Target = dockerBuildTarget
		} else if dockerBuildTarget := cmdCtx.AppConfig.DockerBuildTarget(); dockerBuildTarget != "" {
			opts.Target = dockerBuildTarget
		}

		extraArgs, err := cmdutil.ParseKVStringsToMap(cmdCtx.Config.GetStringSlice("build-arg"))
		if err != nil {
			return errors.Wrap(err, "invalid build-arg")
		}
		opts.ExtraBuildArgs = extraArgs

		img, err = resolver.BuildImage(ctx, cmdCtx.IO, opts)
		if err != nil {
			return err
		}
		if img == nil {
			return errors.New("could not find an image to deploy")
		}
	}

	if img == nil {
		return errors.New("could not find an image to deploy")
	}

	fmt.Fprintf(cmdCtx.Client.IO.Out, "Image: %s\n", img.Tag)
	fmt.Fprintf(cmdCtx.Client.IO.Out, "Image size: %s\n", humanize.Bytes(uint64(img.Size)))

	if cmdCtx.Config.GetBool("build-only") {
		return nil
	}

	cmdfmt.PrintBegin(cmdCtx.Out, "Creating release")

	input := api.DeployImageInput{
		AppID: cmdCtx.AppName,
		Image: img.Tag,
	}
	if val := cmdCtx.Config.GetString("strategy"); val != "" {
		input.Strategy = api.StringPointer(strings.ToUpper(val))
	}
	if cmdCtx.AppConfig != nil && len(cmdCtx.AppConfig.Definition) > 0 {
		input.Definition = api.DefinitionPtr(cmdCtx.AppConfig.Definition)
	}

	release, releaseCommand, err := cmdCtx.Client.API().DeployImage(ctx, input)
	if err != nil {
		return err
	}

	fmt.Fprintf(cmdCtx.Out, "Release v%d created\n", release.Version)
	if releaseCommand != nil {
		fmt.Fprintf(cmdCtx.Out, "Release command detected: this new release will not be available until the command succeeds.\n")
	}

	if cmdCtx.Config.GetBool("detach") {
		return nil
	}

	fmt.Println()
	cmdCtx.Status("deploy", cmdctx.SDETAIL, "You can detach the terminal anytime without stopping the deployment")

	if releaseCommand != nil {
		cmdfmt.PrintBegin(cmdCtx.Out, "Release command")
		fmt.Printf("Command: %s\n", releaseCommand.Command)

		err = watchReleaseCommand(ctx, cmdCtx, cmdCtx.Client.API(), releaseCommand.ID)
		if err != nil {
			return err
		}
	}

	if release.DeploymentStrategy == "IMMEDIATE" {
		terminal.Debug("immediate deployment strategy, nothing to monitor")
		return nil
	}

	return watchDeployment(ctx, cmdCtx)
}

func watchReleaseCommand(ctx context.Context, cc *cmdctx.CmdContext, apiClient *api.Client, id string) error {
	g, ctx := errgroup.WithContext(ctx)
	interactive := cc.IO.IsInteractive()

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Writer = os.Stderr
	s.Prefix = "Running release task..."

	if interactive {
		s.Start()
		defer s.Stop()
	}

	rcUpdates := make(chan api.ReleaseCommand)

	var once sync.Once

	startLogs := func(ctx context.Context, vmid string) {
		once.Do(func() {
			g.Go(func() error {
				ctx, cancel := context.WithCancel(ctx)
				defer cancel()

				opts := &logs.LogOptions{MaxBackoff: 1 * time.Second, AppName: cc.AppName, VMID: vmid}
				ls, err := logs.NewPollingStream(ctx, apiClient, opts)
				if err != nil {
					return err
				}

				for entry := range ls.Stream(ctx, opts) {
					func() {
						if interactive {
							s.Stop()
							defer s.Start()
						}

						fmt.Println("\t", entry.Message)

						// watch for the shutdown message
						if entry.Message == "Starting clean up." {
							cancel()
						}
					}()
				}
				return ls.Err()
			})
		})
	}

	g.Go(func() error {
		var lastValue *api.ReleaseCommand
		var errorCount int
		defer close(rcUpdates)

		for {
			rc, err := apiClient.GetReleaseCommand(ctx, id)
			if err != nil {
				errorCount += 1
				if errorCount < 3 {
					continue
				}
				return err
			}

			if !reflect.DeepEqual(lastValue, rc) {
				lastValue = rc
				rcUpdates <- *rc
			}

			if !rc.InProgress {
				break
			}

			time.Sleep(500 * time.Millisecond)
		}

		return nil
	})

	g.Go(func() error {
		// The logs goroutine will stop itself when it sees a shutdown log message.
		// If the message never comes (delayed logs, etc) the deploy will hang.
		// This timeout makes sure they always stop a few seconds after the release task is done.
		logsCtx, logsCancel := context.WithCancel(ctx)
		defer time.AfterFunc(3*time.Second, logsCancel)

		for rc := range rcUpdates {
			if interactive {
				s.Prefix = fmt.Sprintf("Running release task (%s)...", rc.Status)
			}

			if rc.InstanceID != nil {
				startLogs(logsCtx, *rc.InstanceID)
			}

			if !rc.InProgress && rc.Failed {
				if rc.Succeeded && interactive {
					s.FinalMSG = "Running release task...Done\n"
				} else if rc.Failed {
					return errors.New("Release command failed, deployment aborted")
				}
			}
		}

		return nil
	})

	return g.Wait()
}

func watchDeployment(ctx context.Context, cmdCtx *cmdctx.CmdContext) error {

	return nil
}
