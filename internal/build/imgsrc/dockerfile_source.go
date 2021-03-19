package imgsrc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/jsonmessage"
	"github.com/pkg/errors"
	"github.com/superfly/flyctl/docker"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/pkg/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type RegistryUnauthorizedError struct {
	Tag string
}

func (err *RegistryUnauthorizedError) Error() string {
	return fmt.Sprintf("you are not authorized to push \"%s\"", err.Tag)
}

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
		dockerfile = docker.ResolveDockerfile(opts.WorkingDir)
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

	fmt.Println("building archive")
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
	fmt.Println("building archive done")

	var imageID string

	fmt.Println("building image")
	options := types.ImageBuildOptions{
		Tags:      []string{opts.Tag},
		BuildArgs: normalizeBuildArgsForDocker(opts.AppConfig, opts.ExtraBuildArgs),
		// NoCache:   true,
		AuthConfigs: AuthConfigs(),
		Platform:    "linux/amd64",
	}

	resp, err := docker.ImageBuild(ctx, r, options)
	if err != nil {
		return nil, errors.Wrap(err, "error building with docker")
	}
	defer resp.Body.Close()

	idCallback := func(m jsonmessage.JSONMessage) {
		var aux types.BuildResult

		if err := json.Unmarshal(*m.Aux, &aux); err != nil {
			fmt.Println("error unmarshalling id")
		}

		imageID = aux.ID
	}

	if err := jsonmessage.DisplayJSONMessagesStream(resp.Body, streams.ErrOut, streams.StderrFd(), streams.IsStderrTTY(), idCallback); err != nil {
		return nil, errors.Wrap(err, "error rendering build status stream")
	}

	fmt.Println("building image done")

	if opts.Publish {
		fmt.Println("pushing image")

		registryAuth := FlyRegistryAuth()
		pushResp, err := docker.ImagePush(ctx, opts.Tag, types.ImagePushOptions{
			RegistryAuth: registryAuth,
		})
		if err != nil {
			return nil, errors.Wrap(err, "error pushing image to registry")
		}
		defer pushResp.Close()

		err = jsonmessage.DisplayJSONMessagesStream(pushResp, streams.ErrOut, streams.StderrFd(), streams.IsStderrTTY(), nil)
		if err != nil {
			var msgerr *jsonmessage.JSONError

			if errors.As(err, &msgerr) {
				if msgerr.Message == "denied: requested access to the resource is denied" {
					return nil, &RegistryUnauthorizedError{
						Tag: opts.Tag,
					}
				}
			}
			return nil, errors.Wrap(err, "error rendering push status stream")
		}

		fmt.Println("pushing image done ")
	}

	img, _, err := docker.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return nil, errors.Wrap(err, "count not find built image")
	}
	fmt.Println(img)

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
