//go:build integration
// +build integration

package deployer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/testlib"
)

func TestDeployerDockerfile(t *testing.T) {
	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		panic(err)
	}
	defer dockerClient.Close()

	f := testlib.NewTestEnvFromEnv(t)

	err = testlib.CopyFixtureIntoWorkDir(f.WorkDir(), "deploy-node")
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

	imageRef := os.Getenv("FLY_DEPLOYER_IMAGE")
	require.NotEmpty(t, imageRef)

	if os.Getenv("FLY_DEPLOYER_IMAGE_NO_PULL") == "" {
		fmt.Println("pulling image...")
		out, err := dockerClient.ImagePull(ctx, imageRef, image.PullOptions{Platform: "linux/amd64"})
		if err != nil {
			panic(err)
		}

		defer out.Close()

		_, err = io.Copy(os.Stdout, out)
		if err != nil {
			// TODO: fatal?
			fmt.Printf("error copying image pull io: %v\n", err)
		}
	}

	fmt.Printf("creating container... binding /usr/src/app to %s\n", f.WorkDir())
	cont, err := dockerClient.ContainerCreate(ctx, &container.Config{
		Image: imageRef,
		Env: []string{
			fmt.Sprintf("FLY_API_TOKEN=%s", f.AccessToken()),
			fmt.Sprintf("DEPLOY_ORG_SLUG=%s", f.OrgSlug()),
			"DEPLOY_ONLY=1",
			"DEPLOY_NOW=1",
		},
		Tty: false,
	}, &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyDisabled,
		},
		Binds:       []string{fmt.Sprintf("%s:/usr/src/app", f.WorkDir())},
		NetworkMode: network.NetworkHost,
	}, nil, &v1.Platform{
		Architecture: "amd64",
		OS:           "linux",
	}, fmt.Sprintf("deployer-%s", appName))

	if err != nil {
		panic(err)
	}

	fmt.Printf("Container %s is created\n", cont.ID)

	defer dockerClient.ContainerRemove(ctx, cont.ID, container.RemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   true,
		Force:         true,
	})

	fmt.Println("starting container...")
	err = dockerClient.ContainerStart(ctx, cont.ID, container.StartOptions{})
	if err != nil {
		panic(err)
	}

	logs, err := dockerClient.ContainerLogs(context.Background(), cont.ID, container.LogsOptions{
		ShowStderr: true,
		ShowStdout: true,
		Follow:     true,
	})
	if err != nil {
		panic(err)
	}

	defer logs.Close()

	waitCh, waitErrCh := dockerClient.ContainerWait(ctx, cont.ID, container.WaitConditionNotRunning)

	logCh := make(chan *log)

	go func() {
		var err error
		hdr := make([]byte, 8)
		for {
			// var n int
			_, err = logs.Read(hdr)
			// fmt.Printf("read %d bytes of logs\n", n)
			if err != nil {
				if errors.Is(err, io.EOF) {
					// fmt.Println("EOF!")
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

	msgDone := false
	exited := false
	var exitCode int64

	dep := DeployerOut{Artifacts: map[string]json.RawMessage{}}

	for {
		if err != nil || (exited && msgDone) {
			fmt.Printf("container done, code: %d, error: %+v\n", exitCode, err)
			break
		}
		select {
		case l := <-logCh:
			msgDone = l == nil
			if !msgDone {
				var msg Message

				fmt.Print(string(l.data))

				if len(l.data) > 0 {
					err = json.Unmarshal(l.data, &msg)
					if err == nil {
						if msg.Step != "" {
							found := false
							for _, s := range dep.Steps {
								if s == msg.Step {
									found = true
									break
								}
							}
							if !found {
								dep.Steps = append(dep.Steps, msg.Step)
							}
						}

						if artifactName := strings.TrimPrefix(msg.Type, "artifact:"); artifactName != msg.Type {
							dep.Artifacts[artifactName] = msg.Payload
						}

						dep.Messages = append(dep.Messages, msg)
					}
				}
			}
		case w := <-waitCh:
			exited = true
			exitCode = w.StatusCode
			if w.Error != nil {
				err = errors.New(w.Error.Message)
			}
		case we := <-waitErrCh:
			exited = true
			err = we
		}
	}

	require.Nil(t, err)
	require.Zero(t, exitCode)

	body, err := testlib.RunHealthCheck(fmt.Sprintf("https://%s.fly.dev", appName))
	require.NoError(t, err)

	require.Contains(t, string(body), fmt.Sprintf("Hello, World! %s", f.ID()))

	var meta ArtifactMeta
	err = json.Unmarshal(dep.Artifacts["meta"], &meta)
	require.NoError(t, err)

	stepNames := make([]string, len(meta.Steps)+1)
	stepNames[0] = "__root__"
	for i, step := range meta.Steps {
		stepNames[i+1] = step.ID
	}

	require.Equal(t, dep.Steps, stepNames)
}

type log struct {
	stream uint8
	data   []byte
}

type Message struct {
	ID   int     `json:"id"`
	Step string  `json:"step"`
	Type string  `json:"type"`
	Time float64 `json:"time"`

	Payload json.RawMessage `json:"payload"`
}

type DeployerOut struct {
	Messages  []Message
	Steps     []string
	Artifacts map[string]json.RawMessage
}

type Step struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type ArtifactMeta struct {
	Steps []Step `json:"steps"`
}
