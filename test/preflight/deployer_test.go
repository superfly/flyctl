//go:build integration
// +build integration

package preflight

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestDeployerDockerfile(t *testing.T) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv)
	if err != nil {
		panic(err)
	}

	f := testlib.NewTestEnvFromEnv(t)

	err = copyFixtureIntoWorkDir(f.WorkDir(), "deploy-node", []string{})
	require.NoError(t, err)

	flyTomlPath := fmt.Sprintf("%s/fly.toml", f.WorkDir())

	appName := f.CreateRandomAppName()
	require.NotEmpty(t, appName)

	err = testlib.OverwriteConfig(flyTomlPath, map[string]any{
		"app":    appName,
		"region": f.PrimaryRegion(),
		"env": map[string]string{
			"TEST_ID": f.ID(),
		},
	})
	require.NoError(t, err)

	// app required
	f.Fly("apps create %s -o %s", appName, f.OrgSlug())

	ctx := context.TODO()

	fmt.Println("creating container...")
	cont, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Hostname: "deployer",
		Image:    "fly-deployer",
		Env: []string{
			fmt.Sprintf("FLY_API_TOKEN=%s", f.AccessToken()),
			fmt.Sprintf("DEPLOY_ORG_SLUG=%s", f.OrgSlug()),
			"DEPLOY_ONLY=1",
		},
		AttachStdout: true,
		AttachStderr: true,
	}, &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyDisabled,
		},
		Binds: []string{fmt.Sprintf("%s:/usr/src/app", f.WorkDir())},
	}, &network.NetworkingConfig{
		EndpointsConfig: map[string]*network.EndpointSettings{},
	}, nil, fmt.Sprintf("deployer-%s", appName))

	if err != nil {
		panic(err)
	}

	logs, err := dockerClient.ContainerLogs(context.Background(), cont.ID, container.LogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Timestamps: false,
		Follow:     true,
		Tail:       "40",
	})
	if err != nil {
		panic(err)
	}

	defer logs.Close()

	fmt.Println("starting container...")
	err = dockerClient.ContainerStart(ctx, cont.ID, container.StartOptions{})
	if err != nil {
		panic(err)
	}

	fmt.Printf("Container %s is created\n", cont.ID)

	defer dockerClient.ContainerRemove(ctx, cont.ID, container.RemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   true,
		Force:         true,
	})

	waitCh, waitErrCh := dockerClient.ContainerWait(ctx, cont.ID, container.WaitConditionNotRunning)

	logCh := make(chan *log)

	go func() {

		hdr := make([]byte, 8)
		for {
			_, err = logs.Read(hdr)
			if err != nil {
				if errors.Is(err, io.EOF) {
					fmt.Println("EOF!")
					logCh <- nil
					break
				}
				panic(err)
			}

			count := binary.BigEndian.Uint32(hdr[4:])
			dat := make([]byte, count)
			_, err = logs.Read(dat)

			logCh <- &log{stream: hdr[0], data: dat}
		}

	}()

	logDone := false
	exited := false
	var exitCode int64
	var exitError error

	for {
		if exited && logDone {
			fmt.Printf("container done, code: %d, error: %+v\n", exitCode, exitError)
			break
		}
		select {
		case l := <-logCh:
			logDone = l == nil
			if !logDone {
				// var w io.Writer
				// switch l.stream {
				// case 1:
				// 	w = os.Stdout
				// default:
				// 	w = os.Stderr
				// }

				fmt.Printf(string(l.data))
			}
		case w := <-waitCh:
			exited = true
			exitCode = w.StatusCode
			if w.Error != nil {
				exitError = errors.New(w.Error.Message)
			}
		case we := <-waitErrCh:
			exited = true
			exitError = we
		}
	}

}

type log struct {
	stream uint8
	data   []byte
}
