package imgsrc

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/state"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/proxy"
	"github.com/superfly/flyctl/terminal"
)

const nixpackInstallerURL string = "https://raw.githubusercontent.com/railwayapp/nixpacks/master/install.sh"

type nixpacksBuilder struct{}

func (*nixpacksBuilder) Name() string {
	return "Nixpacks"
}

func ensureNixpacksBinary(ctx context.Context, streams *iostreams.IOStreams) error {
	// TODO: While this is true now, with XDG base dirs "bin/" will not be
	// inside the config directory.
	binDir := filepath.Join(state.ConfigDirectory(ctx), "bin")

	_, err := os.Stat(filepath.Join(binDir, "nixpacks"))
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	tmpdir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)
	installPath := filepath.Join(tmpdir, "install.sh")

	err = func() error {
		out, err := os.Create(installPath)
		if err != nil {
			return err
		}
		defer func() {
			err := out.Close()
			if err != nil {
				terminal.Debugf("error closing install.sh: %v", err)
			}
		}()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, nixpackInstallerURL, http.NoBody)
		if err != nil {
			return err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close() // skipcq: GO-S2307

		n, err := io.Copy(out, resp.Body)
		if err != nil {
			return err
		}
		terminal.Debugf("copied %d bytes to %s\n", n, installPath)
		return nil
	}()

	if err != nil {
		return err
	}

	if err := os.MkdirAll(binDir, 0o700); err != nil {
		return errors.Wrapf(err, "could not create directory at %s", binDir)
	}

	cmd := exec.CommandContext(ctx, "bash", installPath, "--bin-dir", binDir)
	cmd.Stdout = streams.Out
	cmd.Stderr = streams.ErrOut
	cmd.Stdin = nil

	if err := cmd.Run(); err != nil {
		return err
	}

	return err
}

func (*nixpacksBuilder) Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	build.BuildStart()
	if !dockerFactory.mode.IsAvailable() {
		note := "docker daemon not available, skipping"
		terminal.Debug(note)
		build.BuildFinish()
		return nil, note, nil
	}

	if err := ensureNixpacksBinary(ctx, streams); err != nil {
		build.BuildFinish()
		return nil, "", errors.Wrap(err, "could not install nixpacks")
	}

	build.BuilderInitStart()
	docker, err := dockerFactory.buildFn(ctx, build)
	if err != nil {
		build.BuilderInitFinish()
		build.BuildFinish()
		return nil, "", err
	}
	defer docker.Close() // skipcq: GO-S2307

	dockerHost := docker.DaemonHost()

	if dockerFactory.IsRemote() {
		agentclient, err := agent.Establish(ctx, dockerFactory.apiClient)
		if err != nil {
			build.BuilderInitFinish()
			build.BuildFinish()
			return nil, "", err
		}

		machine, app, err := remoteBuilderMachine(ctx, dockerFactory.apiClient, dockerFactory.appName)
		if err != nil {
			build.BuilderInitFinish()
			build.BuildFinish()
			return nil, "", err
		}

		var remoteHost string
		for _, ip := range machine.IPs.Nodes {
			terminal.Debugf("checking ip %+v\n", ip)
			if ip.Kind == "privatenet" {
				remoteHost = ip.IP
				break
			}
		}

		if remoteHost == "" {
			build.BuilderInitFinish()
			build.BuildFinish()
			return nil, "", fmt.Errorf("could not find machine IP")
		}

		dialer, err := agentclient.ConnectToTunnel(ctx, app.Organization.Slug)
		if err != nil {
			build.BuilderInitFinish()
			build.BuildFinish()
			return nil, "", err
		}

		tmpdir, err := os.MkdirTemp("", "")
		if err != nil {
			build.BuilderInitFinish()
			build.BuildFinish()
			return nil, "", err
		}

		defer os.RemoveAll(tmpdir)

		sockPath := filepath.Join(tmpdir, "docker.sock")

		params := &proxy.ConnectParams{
			Ports:            []string{sockPath, "2375"},
			AppName:          app.Name,
			OrganizationSlug: app.Organization.Slug,
			Dialer:           dialer,
			PromptInstance:   false,
			RemoteHost:       remoteHost,
		}

		dockerHost = fmt.Sprintf("unix://%s", sockPath)

		server, err := proxy.NewServer(ctx, params)
		if err != nil {
			build.BuilderInitFinish()
			build.BuildFinish()
			return nil, "", err
		}

		go server.ProxyServer(ctx)
		time.Sleep(50 * time.Millisecond)
	}

	defer clearDeploymentTags(ctx, docker, opts.Tag)
	build.BuilderInitFinish()

	build.ImageBuildStart()

	// TODO: While this is true now, with XDG base dirs "bin/" will not be
	// inside the config directory.
	confDir := state.ConfigDirectory(ctx)
	nixpacksPath := filepath.Join(confDir, "bin", "nixpacks")

	nixpacksArgs := []string{"build", "--name", opts.Tag, opts.WorkingDir}
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "NIXPACKS_") {
			nixpacksArgs = append(nixpacksArgs, "--env", kv)
		}
	}

	terminal.Debugf("calling nixpacks at %s with args: %v and docker host: %s", nixpacksPath, nixpacksArgs, dockerHost)

	cmd := exec.CommandContext(ctx, nixpacksPath, nixpacksArgs...)
	cmd.Env = append(cmd.Env, fmt.Sprintf("DOCKER_HOST=%s", dockerHost), fmt.Sprintf("PATH=%s", os.Getenv("PATH")))
	cmd.Stdout = streams.Out
	cmd.Stderr = streams.ErrOut
	cmd.Stdin = nil

	if err := cmd.Run(); err != nil {
		build.ImageBuildFinish()
		build.BuildFinish()
		return nil, "", err
	}
	build.ImageBuildFinish()
	build.BuildFinish()

	build.PushStart()
	if err := pushToFly(ctx, docker, streams, opts.Tag); err != nil {
		build.PushFinish()
		return nil, "", err
	}
	build.PushFinish()

	img, err := findImageWithDocker(ctx, docker, opts.Tag)
	if err != nil {
		return nil, "", err
	}

	return &DeploymentImage{
		ID:   img.ID,
		Tag:  opts.Tag,
		Size: img.Size,
	}, "", nil
}
