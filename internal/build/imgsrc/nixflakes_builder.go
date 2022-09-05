package imgsrc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"os"
	"os/exec"
)

type nixBuildOutput struct {
	DerivationPath string            `json:"drvPath"`
	Outputs        map[string]string `json:"outputs"`
}

type nixflakesBuilder struct{}

func (*nixflakesBuilder) Name() string {
	return "Nix Flakes"
}

func (*nixflakesBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error) {
	if !dockerFactory.mode.IsAvailable() {
		terminal.Debug("docker daemon not available, skipping")
		return nil, nil
	}

	docker, err := dockerFactory.buildFn(ctx)
	if err != nil {
		return nil, err
	}

	dockerHost := docker.DaemonHost()

	defer clearDeploymentTags(ctx, docker, opts.Tag)

	nixArgs := []string{"build", "--no-link", "--json"}
	if buildAttr, ok := opts.BuildArgs["attr"]; ok {
		nixArgs = append(nixArgs, fmt.Sprintf(".#%s", buildAttr))
	}

	terminal.Debugf("calling nix with args: %v and docker host: %s", nixArgs, dockerHost)

	nixOutput := new(bytes.Buffer)
	cmd := exec.CommandContext(ctx, "nix", nixArgs...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_HOST=%s", dockerHost), fmt.Sprintf("PATH=%s", os.Getenv("PATH")))
	cmd.Stdout = nixOutput
	cmd.Stderr = streams.ErrOut
	cmd.Stdin = nil

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	terminal.Debug(nixOutput.String())

	outputs := make([]nixBuildOutput, 0)
	if err := json.Unmarshal(nixOutput.Bytes(), &outputs); err != nil {
		return nil, err
	}

	if len(outputs) != 1 {
		return nil, fmt.Errorf("unexpected number of outputs: %d", len(outputs))
	}

	output := outputs[0]

	return nil, fmt.Errorf("not implemented")

	if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
		return nil, err
	}

	img, err := findImageWithDocker(ctx, docker, opts.Tag)
	if err != nil {
		return nil, err
	}

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, nil
}
