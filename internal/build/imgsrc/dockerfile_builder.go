package imgsrc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"

	"github.com/containerd/console"
	"github.com/docker/docker/api/types"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/docker/docker/pkg/stringid"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/moby/term"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
	"golang.org/x/sync/errgroup"
)

type dockerfileStrategy struct{}

func (ds *dockerfileStrategy) Name() string {
	return "Dockerfile"
}

func (ds *dockerfileStrategy) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error) {
	if !dockerFactory.mode.IsAvailable() {
		terminal.Debug("docker daemon not available, skipping")
		return nil, nil
	}

	var dockerfile string

	if opts.DockerfilePath != "" {
		if !helpers.FileExists(opts.DockerfilePath) {
			return nil, fmt.Errorf("Dockerfile '%s' not found", opts.DockerfilePath)
		}
		dockerfile = opts.DockerfilePath
	} else {
		dockerfile = resolveDockerfile(opts.WorkingDir)
	}

	if dockerfile == "" {
		terminal.Debug("dockerfile not found, skipping")
		return nil, nil
	}

	docker, err := dockerFactory.buildFn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error connecting to docker")
	}

	defer clearDeploymentTags(ctx, docker, opts.Tag)

	cmdfmt.PrintBegin(streams.ErrOut, "Creating build context")
	archiveOpts := archiveOptions{
		sourcePath: opts.WorkingDir,
		compressed: dockerFactory.mode.IsRemote(),
	}

	excludes, err := readDockerignore(opts.WorkingDir)
	if err != nil {
		return nil, errors.Wrap(err, "error reading .dockerignore")
	}
	archiveOpts.exclusions = excludes

	// copy dockerfile into the archive if it's outside the context dir
	if !isPathInRoot(dockerfile, opts.WorkingDir) {
		dockerfileData, err := os.ReadFile(dockerfile)
		if err != nil {
			return nil, errors.Wrap(err, "error reading Dockerfile")
		}
		archiveOpts.additions = map[string][]byte{
			"Dockerfile": dockerfileData,
		}
	}

	r, err := archiveDirectory(archiveOpts)
	if err != nil {
		return nil, errors.Wrap(err, "error archiving build context")
	}
	cmdfmt.PrintDone(streams.ErrOut, "Creating build context done")

	var imageID string

	cmdfmt.PrintBegin(streams.ErrOut, "Building image with Docker")

	buildArgs := normalizeBuildArgsForDocker(opts.AppConfig, opts.ExtraBuildArgs)

	buildkitEnabled, err := buildkitEnabled(docker)
	terminal.Debugf("buildkitEnabled", buildkitEnabled)
	if err != nil {
		return nil, errors.Wrap(err, "error checking for buildkit support")
	}
	if buildkitEnabled {
		imageID, err = runBuildKitBuild(ctx, streams, docker, r, opts, buildArgs)
		if err != nil {
			return nil, errors.Wrap(err, "error building")
		}
	} else {
		imageID, err = runClassicBuild(ctx, streams, docker, r, opts, buildArgs)
		if err != nil {
			return nil, errors.Wrap(err, "error building")
		}
	}

	cmdfmt.PrintDone(streams.ErrOut, "Building image done")

	if opts.Publish {
		cmdfmt.PrintBegin(streams.ErrOut, "Pushing image to fly")

		if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
			return nil, err
		}

		cmdfmt.PrintDone(streams.ErrOut, "Pushing image done")
	}

	img, _, err := docker.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return nil, errors.Wrap(err, "count not find built image")
	}

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, nil
}

func normalizeBuildArgsForDocker(appConfig *flyctl.AppConfig, extra map[string]string) map[string]*string {
	var out = map[string]*string{}

	if appConfig.Build != nil {
		for k, v := range appConfig.Build.Args {
			// docker needs a string pointer. since ranges reuse variables we need to deref a copy
			val := v
			out[k] = &val
		}
	}

	for name, value := range extra {
		// docker needs a string pointer. since ranges reuse variables we need to deref a copy
		val := value
		out[name] = &val
	}

	return out
}

func runClassicBuild(ctx context.Context, streams *iostreams.IOStreams, docker *dockerclient.Client, r io.ReadCloser, opts ImageOptions, buildArgs map[string]*string) (imageID string, err error) {
	options := types.ImageBuildOptions{
		Tags:      []string{opts.Tag},
		BuildArgs: buildArgs,
		// NoCache:   true,
		AuthConfigs: authConfigs(),
		Platform:    "linux/amd64",
	}

	resp, err := docker.ImageBuild(ctx, r, options)
	if err != nil {
		return "", errors.Wrap(err, "error building with docker")
	}
	defer resp.Body.Close()

	idCallback := func(m jsonmessage.JSONMessage) {
		fmt.Println("got a message", m.ID, m)
		var aux types.BuildResult
		if err := json.Unmarshal(*m.Aux, &aux); err != nil {
			fmt.Fprintf(streams.Out, "failed to parse aux message: %v", err)
		}
		imageID = aux.ID
	}

	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, streams.ErrOut, streams.StderrFd(), streams.IsStderrTTY(), idCallback); err != nil {
		return "", errors.Wrap(err, "error rendering build status stream")
	}

	return imageID, nil
}

const uploadRequestRemote = "upload-request"

func runBuildKitBuild(ctx context.Context, streams *iostreams.IOStreams, docker *dockerclient.Client, r io.ReadCloser, opts ImageOptions, buildArgs map[string]*string) (imageID string, err error) {
	s, err := createBuildSession(opts.WorkingDir)
	if err != nil {
		panic(err)
	}

	if s == nil {
		panic("buildkit not supported")
	}

	eg, errCtx := errgroup.WithContext(ctx)

	dialSession := func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
		return docker.DialHijack(errCtx, "/session", proto, meta)
	}
	eg.Go(func() error {
		return s.Run(context.TODO(), dialSession)
	})

	buildID := stringid.GenerateRandomID()
	eg.Go(func() error {
		buildOptions := types.ImageBuildOptions{
			Version: types.BuilderBuildKit,
			BuildID: uploadRequestRemote + ":" + buildID,
		}

		response, err := docker.ImageBuild(context.Background(), r, buildOptions)
		if err != nil {
			return err
		}
		defer response.Body.Close()
		return nil
	})

	eg.Go(func() error {
		defer s.Close()

		buildOpts := types.ImageBuildOptions{
			Tags:          []string{opts.Tag},
			BuildArgs:     buildArgs,
			Version:       types.BuilderBuildKit,
			AuthConfigs:   authConfigs(),
			SessionID:     s.ID(),
			RemoteContext: uploadRequestRemote,
			BuildID:       buildID,
			Platform:      "linux/amd64",
		}

		return func() error {
			resp, err := docker.ImageBuild(ctx, nil, buildOpts)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			done := make(chan struct{})
			defer close(done)

			eg.Go(func() error {
				select {
				case <-ctx.Done():
					return docker.BuildCancel(context.TODO(), buildOpts.BuildID)
				case <-done:
				}
				return nil
			})

			// TODO: replace with iostreams
			termFd, isTerm := term.GetFdInfo(os.Stderr)
			tracer := newTracer()
			var c2 console.Console
			if isTerm {
				if cons, err := console.ConsoleFromFile(os.Stderr); err == nil {
					c2 = cons
				}
			}

			eg.Go(func() error {
				return progressui.DisplaySolveStatus(context.TODO(), "", c2, os.Stderr, tracer.displayCh)
			})

			auxCallback := func(m jsonmessage.JSONMessage) {
				if m.ID == "moby.image.id" {
					var result types.BuildResult
					if err := json.Unmarshal(*m.Aux, &result); err != nil {
						fmt.Fprintf(streams.Out, "failed to parse aux message: %v", err)
					}
					imageID = result.ID
					return
				}

				tracer.write(m)
			}
			defer close(tracer.displayCh)

			buf := bytes.NewBuffer(nil)

			if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, buf, termFd, isTerm, auxCallback); err != nil {
				return err
			}

			return nil
		}()
	})

	if err := eg.Wait(); err != nil {
		return "", err
	}

	return imageID, nil
}

func pushToFly(ctx context.Context, docker *dockerclient.Client, streams *iostreams.IOStreams, tag string) error {
	pushResp, err := docker.ImagePush(ctx, tag, types.ImagePushOptions{
		RegistryAuth: flyRegistryAuth(),
	})
	if err != nil {
		return errors.Wrap(err, "error pushing image to registry")
	}
	defer pushResp.Close()

	err = jsonmessage.DisplayJSONMessagesStream(pushResp, streams.ErrOut, streams.StderrFd(), streams.IsStderrTTY(), nil)
	if err != nil {
		var msgerr *jsonmessage.JSONError

		if errors.As(err, &msgerr) {
			if msgerr.Message == "denied: requested access to the resource is denied" {
				return &RegistryUnauthorizedError{Tag: tag}
			}
		}
		return errors.Wrap(err, "error rendering push status stream")
	}

	return nil
}
