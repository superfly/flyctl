package imgsrc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/tlsconfig"
	"github.com/jpillora/backoff"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/terminal"
)

type dockerClientFactory struct {
	mode    DockerDaemonType
	buildFn func(ctx context.Context) (*dockerclient.Client, error)
}

func newDockerClientFactory(daemonType DockerDaemonType, apiClient *api.Client, appName string) *dockerClientFactory {
	if daemonType.AllowLocal() {
		c, err := newLocalDockerClient()
		if c != nil && err == nil {
			return &dockerClientFactory{
				mode: DockerDaemonTypeLocal,
				buildFn: func(ctx context.Context) (*dockerclient.Client, error) {
					return c, nil
				},
			}
		} else if err != nil {
			terminal.Warn("Error connecting to local docker daemon:", err)
		} else {
			terminal.Warn("Local docker daemon not present")
		}
	}

	if daemonType.AllowRemote() {
		var cachedDocker *dockerclient.Client

		return &dockerClientFactory{
			mode: DockerDaemonTypeRemote,
			buildFn: func(ctx context.Context) (*dockerclient.Client, error) {
				if cachedDocker != nil {
					return cachedDocker, nil
				}
				c, err := newRemoteDockerClient(ctx, apiClient, appName)
				if err != nil {
					return nil, err
				}
				cachedDocker = c
				return cachedDocker, nil
			},
		}
	}

	return &dockerClientFactory{
		mode: DockerDaemonTypeNone,
		buildFn: func(ctx context.Context) (*dockerclient.Client, error) {
			return nil, errors.New("no docker daemon available")
		},
	}
}

var unauthorizedError = errors.New("You are unauthorized to use this builder")

func isUnauthorized(err error) bool {
	return errors.Is(err, unauthorizedError)
}

func isRetyableError(err error) bool {
	return !isUnauthorized(err)
}

type DockerDaemonType int

const (
	DockerDaemonTypeLocal DockerDaemonType = 1 << iota
	DockerDaemonTypeRemote
	DockerDaemonTypeNone
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

func (t DockerDaemonType) IsLocal() bool {
	return t == DockerDaemonTypeLocal
}

func (t DockerDaemonType) IsRemote() bool {
	return t == DockerDaemonTypeRemote
}

func (t DockerDaemonType) IsNone() bool {
	return t == DockerDaemonTypeNone
}

func (t DockerDaemonType) IsAvailable() bool {
	return !t.IsNone()
}

func newLocalDockerClient() (*dockerclient.Client, error) {
	c, err := dockerclient.NewClientWithOpts(dockerclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}

	if err := dockerclient.FromEnv(c); err != nil {
		return nil, err
	}

	return c, nil
}

func newRemoteDockerClient(ctx context.Context, apiClient *api.Client, appName string) (*dockerclient.Client, error) {
	host, err := remoteBuilderURL(apiClient, appName)
	if err != nil {
		return nil, err
	}

	terminal.Debugf("Remote Docker builder host: %s\n", host)

	httpc := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsconfig.ClientDefault(),
		},
	}

	client, err := dockerclient.NewClientWithOpts(
		dockerclient.WithAPIVersionNegotiation(),
		dockerclient.WithHTTPClient(httpc),
		dockerclient.WithHost(host),
		dockerclient.WithHTTPHeaders(map[string]string{
			"Authorization": basicAuth(appName, flyctl.GetAPIToken()),
		}))

	if err != nil {
		return nil, errors.Wrap(err, "Error creating docker client")
	}

	terminal.Infof("Waiting for remote builder to become available...\n")

	if err := waitForDaemon(ctx, client); err != nil {
		return nil, err
	}

	return client, nil
}

func remoteBuilderURL(apiClient *api.Client, appName string) (string, error) {
	if v := os.Getenv("FLY_REMOTE_BUILDER_HOST"); v != "" {
		return v, nil
	}

	rawURL, _, err := apiClient.EnsureRemoteBuilder(appName)
	if err != nil {
		return "", fmt.Errorf("could not create remote builder: %v", err)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.Wrap(err, "error parsing remote builder url")
	}

	host := parsedURL.Hostname()
	port := parsedURL.Port()

	if port == "" {
		port = "10000"
	}

	return "tcp://" + net.JoinHostPort(host, port), nil
}

func basicAuth(appName, authToken string) string {
	auth := appName + ":" + authToken
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(auth))
}

func waitForDaemon(ctx context.Context, client *dockerclient.Client) error {
	deadline := time.After(5 * time.Minute)

	b := &backoff.Backoff{
		//These are the defaults
		Min:    200 * time.Millisecond,
		Max:    2 * time.Second,
		Factor: 1.2,
		Jitter: true,
	}

	consecutiveSuccesses := 0
	var healthyStart time.Time

OUTER:
	for {
		checkErr := make(chan error, 1)

		go func() {
			_, err := client.Ping(ctx)
			checkErr <- err
		}()

		select {
		case err := <-checkErr:
			if err == nil {
				if consecutiveSuccesses == 0 {
					// reset on the first success in a row so the next checks are a bit spaced out
					healthyStart = time.Now()
					b.Reset()
				}
				consecutiveSuccesses++

				if time.Since(healthyStart) > 3*time.Second {
					terminal.Info("Remote builder is ready to build!")
					break OUTER
				}

				dur := b.Duration()
				terminal.Debugf("Remote builder available, but pinging again in %s to be sure\n", dur)
				time.Sleep(dur)
			} else {
				if !isRetyableError(err) {
					return err
				}
				consecutiveSuccesses = 0
				dur := b.Duration()
				terminal.Debugf("Remote builder unavailable, retrying in %s (err: %v)\n", dur, err)
				time.Sleep(dur)
			}
		case <-deadline:
			return fmt.Errorf("Could not ping remote builder within 5 minutes, aborting.")
		case <-ctx.Done():
			terminal.Warn("Canceled")
			break OUTER
		}
	}

	return nil
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

func newDeploymentTag(appName string, label string) string {
	if tag := os.Getenv("FLY_IMAGE_REF"); tag != "" {
		return tag
	}

	if label == "" {
		label = fmt.Sprintf("deployment-%d", time.Now().Unix())
	}

	registry := viper.GetString(flyctl.ConfigRegistryHost)

	return fmt.Sprintf("%s/%s:%s", registry, appName, label)
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
