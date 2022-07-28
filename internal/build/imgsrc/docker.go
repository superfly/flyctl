package imgsrc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/azazeal/pause"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/command/orgs/builder"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
)

type dockerClientFactory struct {
	mode      DockerDaemonType
	remote    bool
	buildFn   func(ctx context.Context) (*dockerclient.Client, error)
	apiClient *api.Client
	app       *api.AppCompact
}

func newDockerClientFactory(daemonType DockerDaemonType, apiClient *api.Client, app *api.AppCompact, streams *iostreams.IOStreams) *dockerClientFactory {
	remoteFactory := func() *dockerClientFactory {
		terminal.Debug("trying remote docker daemon")
		var cachedDocker *dockerclient.Client

		return &dockerClientFactory{
			mode:   daemonType,
			remote: true,
			buildFn: func(ctx context.Context) (*dockerclient.Client, error) {
				if cachedDocker != nil {
					return cachedDocker, nil
				}
				c, err := newRemoteDockerClient(ctx, apiClient, app, streams)
				if err != nil {
					return nil, err
				}
				cachedDocker = c
				return cachedDocker, nil
			},
			apiClient: apiClient,
			app:       app,
		}
	}

	localFactory := func() *dockerClientFactory {
		terminal.Debug("trying local docker daemon")
		c, err := NewLocalDockerClient()
		if c != nil && err == nil {
			return &dockerClientFactory{
				mode: DockerDaemonTypeLocal,
				buildFn: func(ctx context.Context) (*dockerclient.Client, error) {
					return c, nil
				},
			}
		} else if err != nil && !dockerclient.IsErrConnectionFailed(err) {
			terminal.Warn("Error connecting to local docker daemon:", err)
		} else {
			terminal.Debug("Local docker daemon unavailable")
		}
		return nil
	}

	if daemonType.AllowRemote() && !daemonType.PrefersLocal() {
		return remoteFactory()
	}
	if daemonType.AllowLocal() {
		if c := localFactory(); c != nil {
			return c
		}
	}
	if daemonType.AllowRemote() {
		return remoteFactory()
	}

	return &dockerClientFactory{
		mode: DockerDaemonTypeNone,
		buildFn: func(ctx context.Context) (*dockerclient.Client, error) {
			return nil, errors.New("no docker daemon available")
		},
	}
}

func NewDockerDaemonType(allowLocal, allowRemote, prefersLocal, useNixpacks bool) DockerDaemonType {
	daemonType := DockerDaemonTypeNone
	if allowLocal {
		daemonType = daemonType | DockerDaemonTypeLocal
	}
	if allowRemote {
		daemonType = daemonType | DockerDaemonTypeRemote
	}
	if useNixpacks {
		daemonType = daemonType | DockerDaemonTypeNixpacks
	}
	if prefersLocal {
		daemonType = daemonType | DockerDaemonTypePrefersLocal
	}
	return daemonType
}

type DockerDaemonType int

const (
	DockerDaemonTypeLocal DockerDaemonType = 1 << iota
	DockerDaemonTypeRemote
	DockerDaemonTypeNone
	DockerDaemonTypePrefersLocal
	DockerDaemonTypeNixpacks
)

func (t DockerDaemonType) AllowLocal() bool {
	return (t & DockerDaemonTypeLocal) != 0
}

func (t DockerDaemonType) AllowRemote() bool {
	return (t & DockerDaemonTypeRemote) != 0
}

func (t DockerDaemonType) AllowNone() bool {
	return (t & DockerDaemonTypeNone) != 0
}

func (t DockerDaemonType) IsNone() bool {
	return t == DockerDaemonTypeNone
}

func (t DockerDaemonType) IsAvailable() bool {
	return !t.IsNone()
}

func (t DockerDaemonType) UseNixpacks() bool {
	return (t & DockerDaemonTypeNixpacks) != 0
}

func (t DockerDaemonType) PrefersLocal() bool {
	return (t & DockerDaemonTypePrefersLocal) != 0
}

func NewLocalDockerClient() (*dockerclient.Client, error) {
	c, err := dockerclient.NewClientWithOpts(dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	if err := dockerclient.FromEnv(c); err != nil {
		return nil, err
	}

	if _, err = c.Ping(context.TODO()); err != nil {
		return nil, err
	}

	return c, nil
}

func newRemoteDockerClient(ctx context.Context, apiClient *api.Client, app *api.AppCompact, streams *iostreams.IOStreams) (*dockerclient.Client, error) {
	startedAt := time.Now()

	var host string
	var err error

	machine, err := builder.GetMachine(ctx, app.Organization.Slug)

	if err != nil {
		return nil, err
	}
	remoteBuilderAppName := app.Name
	remoteBuilderOrg := app.Organization.Slug

	if host != "" {
		terminal.Debugf("Remote Docker builder host: %s\n", host)
	}

	if msg := fmt.Sprintf("Waiting for remote builder %s...", remoteBuilderAppName); streams.IsInteractive() {
		streams.StartProgressIndicatorMsg(msg)
	} else {
		fmt.Fprintln(streams.ErrOut, msg)
	}

	captureError := func(err error) {
		// ignore cancelled errors
		if errors.Is(err, context.Canceled) {
			return
		}

		sentry.CaptureException(err,
			sentry.WithTag("feature", "remote-build"),
			sentry.WithContexts(map[string]interface{}{
				"app": map[string]interface{}{
					"name": app.Name,
				},
				"organization": map[string]interface{}{
					"name": remoteBuilderOrg,
				},
				"builder": map[string]interface{}{
					"app_name": remoteBuilderAppName,
				},
				"elapsed": time.Since(startedAt),
			}),
		)
	}

	host = "tcp://[" + machine.PrivateIP + "]:2375"

	if host == "" {
		return nil, errors.New("machine did not have a private IP")
	}

	opts, err := buildRemoteClientOpts(ctx, apiClient, app.Name, host)
	if err != nil {
		streams.StopProgressIndicator()

		err = fmt.Errorf("failed building options: %w", err)
		captureError(err)

		return nil, err
	}

	client, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		streams.StopProgressIndicator()

		err = fmt.Errorf("failed creating docker client: %w", err)
		captureError(err)

		return nil, err
	}

	switch up, err := waitForDaemon(ctx, client); {
	case err != nil:
		streams.StopProgressIndicator()

		err = fmt.Errorf("failed waiting for docker daemon: %w", err)
		captureError(err)

		return nil, err
	case !up:
		streams.StopProgressIndicator()

		terminal.Warnf("Remote builder did not start in time. Check remote builder logs with `flyctl logs -a %s`\n", remoteBuilderAppName)

		return nil, errors.New("remote builder app unavailable")
	default:
		if msg := fmt.Sprintf("Remote builder %s ready", remoteBuilderAppName); streams.IsInteractive() {
			streams.StopProgressIndicatorMsg(msg)
		} else {
			fmt.Fprintln(streams.ErrOut, msg)
		}
	}

	return client, nil
}

func buildRemoteClientOpts(ctx context.Context, apiClient *api.Client, appName, host string) (opts []dockerclient.Opt, err error) {
	opts = []dockerclient.Opt{
		dockerclient.WithAPIVersionNegotiation(),
		dockerclient.WithHost(host),
	}

	if os.Getenv("FLY_REMOTE_BUILDER_HOST_WG") != "" {
		terminal.Debug("connecting to remote docker daemon over host wireguard tunnel")

		return
	}

	var app *api.AppBasic
	if app, err = apiClient.GetAppBasic(ctx, appName); err != nil {
		return nil, fmt.Errorf("error fetching target app: %w", err)
	}

	var agentclient *agent.Client
	if agentclient, err = agent.Establish(ctx, apiClient); err != nil {
		return
	}

	var dialer agent.Dialer
	if dialer, err = agentclient.Dialer(ctx, app.Organization.Slug); err != nil {
		return
	}

	if err = agentclient.WaitForTunnel(ctx, app.Organization.Slug); err == nil {
		opts = append(opts, dockerclient.WithDialContext(dialer.DialContext))
	}

	return
}

func waitForDaemon(parent context.Context, client *dockerclient.Client) (up bool, err error) {
	ctx, cancel := context.WithTimeout(parent, 2*time.Minute)
	defer cancel()

	b := &backoff.Backoff{
		Min:    50 * time.Millisecond,
		Max:    200 * time.Millisecond,
		Factor: 1.2,
		Jitter: true,
	}

	var (
		consecutiveSuccesses int
		healthyStart         time.Time
	)

	for ctx.Err() == nil {
		switch _, err := client.Ping(ctx); err {
		default:
			consecutiveSuccesses = 0

			dur := b.Duration()
			terminal.Debugf("Remote builder unavailable, retrying in %s (err: %v)\n", dur, err)
			pause.For(ctx, dur)
		case nil:
			if consecutiveSuccesses++; consecutiveSuccesses == 1 {
				healthyStart = time.Now()
			}

			if time.Since(healthyStart) > time.Second {
				terminal.Debug("Remote builder is ready to build!")
				return true, nil
			}

			b.Reset()
			dur := b.Duration()
			terminal.Debugf("Remote builder available, but pinging again in %s to be sure\n", dur)
			pause.For(ctx, dur)
		}
	}

	switch {
	case parent.Err() != nil:
		return false, parent.Err()
	default:
		return false, nil
	}
}

func clearDeploymentTags(ctx context.Context, docker *dockerclient.Client, tag string) error {
	filters := filters.NewArgs(filters.Arg("reference", tag))

	images, err := docker.ImageList(ctx, types.ImageListOptions{Filters: filters})
	if err != nil {
		return err
	}

	for _, image := range images {
		for _, tag := range image.RepoTags {
			_, err := docker.ImageRemove(ctx, tag, types.ImageRemoveOptions{PruneChildren: true})
			if err != nil {
				terminal.Debug("Error deleting image", err)
			}
		}
	}

	return nil
}

func registryAuth(token string) types.AuthConfig {
	return types.AuthConfig{
		Username:      "x",
		Password:      token,
		ServerAddress: "registry.fly.io",
	}
}

func authConfigs() map[string]types.AuthConfig {
	authConfigs := map[string]types.AuthConfig{}

	authConfigs["registry.fly.io"] = registryAuth(flyctl.GetAPIToken())

	dockerhubUsername := os.Getenv("DOCKER_HUB_USERNAME")
	dockerhubPassword := os.Getenv("DOCKER_HUB_PASSWORD")

	if dockerhubUsername != "" && dockerhubPassword != "" {
		cfg := types.AuthConfig{
			Username:      dockerhubUsername,
			Password:      dockerhubPassword,
			ServerAddress: "index.docker.io",
		}
		authConfigs["https://index.docker.io/v1/"] = cfg
	}

	return authConfigs
}

func flyRegistryAuth() string {
	accessToken := flyctl.GetAPIToken()
	authConfig := registryAuth(accessToken)
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		terminal.Warn("Error encoding fly registry credentials", err)
		return ""
	}
	return base64.URLEncoding.EncodeToString(encodedJSON)
}

// NewDeploymentTag generates a Docker image reference including the current registry,
// the app name, and a timestamp: registry.fly.io/appname:deployment-$timestamp
func NewDeploymentTag(appName string, label string) string {
	if tag := os.Getenv("FLY_IMAGE_REF"); tag != "" {
		return tag
	}

	if label == "" {
		label = fmt.Sprintf("deployment-%d", time.Now().Unix())
	}

	registry := viper.GetString(flyctl.ConfigRegistryHost)

	return fmt.Sprintf("%s/%s:%s", registry, appName, label)
}

func newCacheTag(appName string) string {
	registry := viper.GetString(flyctl.ConfigRegistryHost)

	return fmt.Sprintf("%s/%s:%s", registry, appName, "cache")
}

// resolveDockerfile - Resolve the location of the dockerfile, allowing for upper and lowercase naming
func resolveDockerfile(cwd string) string {
	dockerfilePath := filepath.Join(cwd, "Dockerfile")
	if helpers.FileExists(dockerfilePath) {
		return dockerfilePath
	}
	dockerfilePath = filepath.Join(cwd, "dockerfile")
	if helpers.FileExists(dockerfilePath) {
		return dockerfilePath
	}
	return ""
}

func (d *dockerClientFactory) IsRemote() bool {
	return d.remote
}

func (d *dockerClientFactory) IsLocal() bool {
	return !d.remote
}
