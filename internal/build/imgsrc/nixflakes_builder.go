package imgsrc

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"io/ioutil"
	"os"
	"os/exec"
)

type nixBuildOutput struct {
	DerivationPath string            `json:"drvPath"`
	Outputs        map[string]string `json:"outputs"`
}

type dockerImageImportOutput struct {
	Status string `json:"status"`
}

type nixflakesBuilder struct{}

func (*nixflakesBuilder) Name() string {
	return "Nix Flakes"
}

func (b *nixflakesBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions) (*DeploymentImage, error) {
	if !dockerFactory.mode.IsAvailable() {
		terminal.Debug("docker daemon not available, skipping")
		return nil, nil
	}

	docker, err := dockerFactory.buildFn(ctx)
	if err != nil {
		return nil, err
	}

	dockerHost := docker.DaemonHost()

	// defer clearDeploymentTags(ctx, docker, opts.Tag)

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
		terminal.Debugf("nix output: %s\n", nixOutput)
		return nil, err
	}

	outputs := make([]nixBuildOutput, 0)
	if err := json.Unmarshal(nixOutput.Bytes(), &outputs); err != nil {
		return nil, err
	}

	if len(outputs) != 1 {
		return nil, fmt.Errorf("unexpected number of outputs: %d", len(outputs))
	}

	reader, err := os.Open(outputs[0].Outputs["out"])
	if err != nil {
		return nil, fmt.Errorf("error opening %q for reading: %v", outputs[0].Outputs["out"], err)
	}

	terminal.Debugf("importing %s\n", outputs[0].DerivationPath)

	importResp, err := docker.ImageImport(ctx, types.ImageImportSource{
		SourceName: "-",
		Source:     reader,
	}, "", types.ImageImportOptions{})

	if err != nil {
		terminal.Debugf("error importing: %v\n", err)
		return nil, err
	}

	importRespBytes, err := ioutil.ReadAll(importResp)
	if err != nil {
		terminal.Debugf("error importing: %v\n", err)
		return nil, err
	}

	var importRespData dockerImageImportOutput
	if err := json.Unmarshal(importRespBytes, &importRespData); err != nil {
		terminal.Debugf("error importing: %v\n", err)
		return nil, err
	}

	terminal.Debugf("successful import: %q\n", importRespData.Status)

	terminal.Debugf("tagging %q as %q\n", importRespData.Status, opts.Tag)

	if err := docker.ImageTag(ctx, importRespData.Status, opts.Tag); err != nil {
		terminal.Debugf("error tagging: %v\n", err)
	}

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
