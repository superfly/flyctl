//go:build integration
// +build integration

package testlib

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/command/launch"
)

type DeployerTestEnv struct {
	*FlyctlTestEnv
	t            testing.TB
	dockerClient *client.Client
	image        string
	noPull       bool
}

func NewDeployerTestEnvFromEnv(ctx context.Context, t testing.TB) (*DeployerTestEnv, error) {
	imageRef := os.Getenv("FLY_DEPLOYER_IMAGE")
	require.NotEmpty(t, imageRef)
	noPull := os.Getenv("FLY_DEPLOYER_IMAGE_NO_PULL") != ""

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	dockerClient.NegotiateAPIVersion(ctx)

	fmt.Printf("docker API version: %s\n", dockerClient.ClientVersion())

	if !noPull {
		fmt.Println("pulling image...")
		out, err := dockerClient.ImagePull(ctx, imageRef, image.PullOptions{Platform: "linux/amd64"})
		if err != nil {
			return nil, err
		}
		defer out.Close()

		_, err = io.Copy(os.Stdout, out)
		if err != nil {
			return nil, err
		}
	}

	return &DeployerTestEnv{FlyctlTestEnv: NewTestEnvFromEnv(t), t: t, dockerClient: dockerClient, image: imageRef, noPull: noPull}, nil
}

func (d *DeployerTestEnv) Close() error {
	return d.dockerClient.Close()
}

func (d *DeployerTestEnv) NewRun(options ...func(*DeployTestRun)) *DeployTestRun {
	run := &DeployTestRun{FlyctlTestEnv: d.FlyctlTestEnv, dockerClient: d.dockerClient, deployerImage: d.image, apiToken: d.FlyctlTestEnv.AccessToken(), orgSlug: d.FlyctlTestEnv.OrgSlug(), containerBinds: []string{}, Extra: make(map[string]interface{}), Cwd: "", FlyTomlPath: "fly.toml"}
	for _, o := range options {
		o(run)
	}
	return run
}

type DeployTestRun struct {
	*FlyctlTestEnv
	dockerClient  *client.Client
	deployerImage string

	// required!
	apiToken string
	orgSlug  string

	appName string
	gitRepo string
	gitRef  string

	region string

	noCustomize    bool
	skipExtensions bool
	copyConfig     bool
	optOutGha      bool
	customizePath  string

	deployOnly bool
	deployNow  bool

	createAndPushBranch bool

	cleanupBeforeExit bool

	containerBinds []string

	containerID string

	waitCh    chan *DeployerOut
	waitErrCh chan error

	exitCode int64

	done bool
	out  *DeployerOut
	err  error

	Extra map[string]interface{}

	Cwd         string
	FlyTomlPath string
}

func WithApp(app string) func(*DeployTestRun) {
	return func(d *DeployTestRun) {
		d.appName = app
	}
}

func WithPreCustomize(customize interface{}) func(*DeployTestRun) {
	b, err := json.Marshal(customize)
	if err != nil {
		panic(err)
	}
	return func(d *DeployTestRun) {
		p := filepath.Join(d.WorkDir(), "customize.json")
		if err := os.WriteFile(p, b, 0666); err != nil {
			panic(err)
		}
		dst := "/opt/customize.json"
		d.containerBinds = append(d.containerBinds, fmt.Sprintf("%s:%s", p, dst))
		d.customizePath = dst
	}
}

func WithGitRepo(repo string) func(*DeployTestRun) {
	return func(d *DeployTestRun) {
		d.gitRepo = repo
	}
}

func WithGitRef(ref string) func(*DeployTestRun) {
	return func(d *DeployTestRun) {
		d.gitRef = ref
	}
}

func WithRegion(region string) func(*DeployTestRun) {
	return func(d *DeployTestRun) {
		d.region = region
	}
}

func WithoutCustomize(d *DeployTestRun) {
	d.noCustomize = true
}

func WithouExtensions(d *DeployTestRun) {
	d.skipExtensions = true
}

func WithCopyConfig(d *DeployTestRun) {
	d.copyConfig = true
}

func OptOutGithubActions(d *DeployTestRun) {
	d.optOutGha = true
}

func DeployOnly(d *DeployTestRun) {
	d.deployOnly = true
}

func DeployNow(d *DeployTestRun) {
	d.deployNow = true
}

func CreateAndPushBranch(d *DeployTestRun) {
	d.createAndPushBranch = true
}

func CleanupBeforeExit(d *DeployTestRun) {
	d.cleanupBeforeExit = true
}

func WithAppSource(src string) func(*DeployTestRun) {
	return func(d *DeployTestRun) {
		d.containerBinds = append(d.containerBinds, fmt.Sprintf("%s:/usr/src/app", src))
	}
}

func (d *DeployTestRun) Start(ctx context.Context) error {
	env := []string{
		fmt.Sprintf("FLY_API_TOKEN=%s", d.apiToken),
		fmt.Sprintf("DEPLOY_ORG_SLUG=%s", d.orgSlug),
		fmt.Sprintf("DEPLOY_TRIGGER=%s", "launch"),
	}

	if d.appName != "" {
		env = append(env, fmt.Sprintf("DEPLOY_APP_NAME=%s", d.appName))
	}
	if d.gitRepo != "" {
		env = append(env, fmt.Sprintf("GIT_REPO=%s", d.gitRepo))
	}
	if d.gitRef != "" {
		env = append(env, fmt.Sprintf("GIT_REF=%s", d.gitRef))
	}

	if d.region != "" {
		env = append(env, fmt.Sprintf("DEPLOY_APP_REGION=%s", d.region))
	}

	if d.noCustomize {
		env = append(env, "NO_DEPLOY_CUSTOMIZE=1")
	}
	if d.skipExtensions {
		env = append(env, "SKIP_EXTENSIONS=1")
	}
	if d.copyConfig {
		env = append(env, "DEPLOY_COPY_CONFIG=1")
	}
	if d.optOutGha {
		env = append(env, "OPT_OUT_GITHUB_ACTIONS=1")
	}

	if d.deployOnly {
		env = append(env, "DEPLOY_ONLY=1")
	}
	if d.deployNow {
		env = append(env, "DEPLOY_NOW=1")
	}
	if d.FlyTomlPath != "fly.toml" {
		env = append(env, fmt.Sprintf("DEPLOYER_FLY_CONFIG_PATH=%s", d.FlyTomlPath))
	}
	if d.Cwd != "" {
		env = append(env, fmt.Sprintf("DEPLOYER_SOURCE_CWD=%s", d.Cwd))
	}

	if d.createAndPushBranch {
		env = append(env, "DEPLOY_CREATE_AND_PUSH_BRANCH=1")
	}

	if d.cleanupBeforeExit {
		env = append(env, "DEPLOYER_CLEANUP_BEFORE_EXIT=1")
	}

	if d.customizePath != "" {
		env = append(env, fmt.Sprintf("DEPLOY_CUSTOMIZE_PATH=%s", d.customizePath))
	}

	fmt.Printf("creating container... image=%s\n", d.deployerImage)
	cont, err := d.dockerClient.ContainerCreate(ctx, &container.Config{
		Image: d.deployerImage,
		Env:   env,
		Tty:   false,
	}, &container.HostConfig{
		RestartPolicy: container.RestartPolicy{
			Name: container.RestartPolicyDisabled,
		},
		Binds:       d.containerBinds,
		NetworkMode: network.NetworkHost,
	}, nil, &v1.Platform{
		Architecture: "amd64",
		OS:           "linux",
	}, "")

	if err != nil {
		return err
	}

	d.containerID = cont.ID

	fmt.Println("starting container...")
	err = d.dockerClient.ContainerStart(ctx, cont.ID, container.StartOptions{})
	if err != nil {
		fmt.Printf("could not start container: %+v\n", err)
		return err
	}

	d.waitCh = make(chan *DeployerOut, 1)
	d.waitErrCh = make(chan error, 1)

	go func() {
		defer d.dockerClient.ContainerRemove(context.TODO(), cont.ID, container.RemoveOptions{
			RemoveVolumes: true,
			RemoveLinks:   true,
			Force:         true,
		})

		defer d.Close()

		logs, err := d.dockerClient.ContainerLogs(context.Background(), cont.ID, container.LogsOptions{
			ShowStderr: true,
			ShowStdout: true,
			Follow:     true,
		})
		if err != nil {
			panic(err)
		}

		defer logs.Close()

		waitCh, waitErrCh := d.dockerClient.ContainerWait(ctx, cont.ID, container.WaitConditionNotRunning)

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
					d.err = err
					d.waitErrCh <- err
					d.done = true
				}

				count := binary.BigEndian.Uint32(hdr[4:])
				dat := make([]byte, count)
				_, err = logs.Read(dat)

				logCh <- &log{stream: hdr[0], data: dat}
			}
		}()

		msgDone := false
		exited := false

		d.out = &DeployerOut{Artifacts: map[string]json.RawMessage{}}

		for {
			if d.done {
				break
			}
			if err != nil || (exited && msgDone) {
				fmt.Printf("container done, code: %d, error: %+v\n", d.exitCode, err)
				if err != nil {
					d.err = err
					d.waitErrCh <- err
				} else {
					d.waitCh <- d.out
				}
				d.done = true
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
								for _, s := range d.out.Steps {
									if s == msg.Step {
										found = true
										break
									}
								}
								if !found {
									d.out.Steps = append(d.out.Steps, msg.Step)
								}
							}

							if artifactName := strings.TrimPrefix(msg.Type, "artifact:"); artifactName != msg.Type {
								d.out.Artifacts[artifactName] = msg.Payload
							}

							d.out.Messages = append(d.out.Messages, msg)
						}
					}
				}
			case w := <-waitCh:
				exited = true
				d.exitCode = w.StatusCode
				if w.Error != nil {
					err = errors.New(w.Error.Message)
				}
			case we := <-waitErrCh:
				exited = true
				err = we
			}
		}
	}()

	return nil
}

func (d *DeployTestRun) Wait() error {
	if d.done {
		if d.err != nil {
			return d.err
		}
		return nil
	}
	select {
	case <-d.waitCh:
		return nil
	case err := <-d.waitErrCh:
		return err
	}
}

func (d *DeployTestRun) ExitCode() int64 {
	return d.exitCode
}

func (d *DeployTestRun) Output() *DeployerOut {
	return d.out
}

func (d *DeployTestRun) Close() error {
	return nil
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

type Step struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type DeployerOut struct {
	Messages  []Message
	Steps     []string
	Artifacts map[string]json.RawMessage
}

func (out *DeployerOut) ArtifactMeta() (*ArtifactMeta, error) {
	var meta ArtifactMeta
	err := json.Unmarshal(out.Artifacts["meta"], &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

type ArtifactMeta struct {
	Steps []Step `json:"steps"`
}

func (m *ArtifactMeta) StepNames() []string {
	stepNames := make([]string, len(m.Steps))
	for i, step := range m.Steps {
		stepNames[i] = step.ID
	}
	return stepNames
}

func (out *DeployerOut) ArtifactManifest() (*launch.LaunchManifest, error) {
	var manifest launch.LaunchManifest
	err := json.Unmarshal(out.Artifacts["manifest"], &manifest)
	if err != nil {
		return nil, err
	}
	return &manifest, nil
}
