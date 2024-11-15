package imgsrc

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/azazeal/pause"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/registry"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/sockets"
	"github.com/jpillora/backoff"
	"github.com/morikuni/aec"
	"github.com/oklog/ulid/v2"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var (
	cachedDocker     *dockerclient.Client
	wglessCompatible bool
)

type dockerClientFactory struct {
	mode      DockerDaemonType
	remote    bool
	buildFn   func(ctx context.Context, build *build) (*dockerclient.Client, error)
	apiClient flyutil.Client
	appName   string
}

func newDockerClientFactory(daemonType DockerDaemonType, apiClient flyutil.Client, appName string, streams *iostreams.IOStreams, connectOverWireguard, recreateBuilder bool) *dockerClientFactory {
	remoteFactory := func() *dockerClientFactory {
		terminal.Debug("trying remote docker daemon")
		return &dockerClientFactory{
			mode:   daemonType,
			remote: true,
			buildFn: func(ctx context.Context, build *build) (*dockerclient.Client, error) {
				return newRemoteDockerClient(ctx, apiClient, appName, streams, build, cachedDocker, connectOverWireguard, recreateBuilder)
			},
			apiClient: apiClient,
			appName:   appName,
		}
	}

	localFactory := func() *dockerClientFactory {
		terminal.Debug("trying local docker daemon")
		c, err := NewLocalDockerClient()
		if c != nil && err == nil {
			return &dockerClientFactory{
				mode: DockerDaemonTypeLocal,
				buildFn: func(ctx context.Context, build *build) (*dockerclient.Client, error) {
					build.SetBuilderMetaPart1(localBuilderType, "", "")
					return c, nil
				},
				appName: appName,
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
		buildFn: func(ctx context.Context, build *build) (*dockerclient.Client, error) {
			return nil, errors.New("no docker daemon available")
		},
	}
}

func NewDockerDaemonType(allowLocal, allowRemote, prefersLocal, useDepot, useNixpacks bool) DockerDaemonType {
	daemonType := DockerDaemonTypeNone
	if allowLocal {
		daemonType = daemonType | DockerDaemonTypeLocal
	}
	if allowRemote {
		daemonType = daemonType | DockerDaemonTypeRemote
	}
	if useDepot {
		daemonType = daemonType | DockerDaemonTypeDepot
	}
	if useNixpacks {
		daemonType = daemonType | DockerDaemonTypeNixpacks
	}
	if prefersLocal && !useDepot {
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
	DockerDaemonTypeDepot
)

func (t DockerDaemonType) String() string {
	strs := []string{}

	if t&DockerDaemonTypeLocal != 0 {
		strs = append(strs, "local")
	}
	if t&DockerDaemonTypeRemote != 0 {
		strs = append(strs, "remote")
	}
	if t&DockerDaemonTypePrefersLocal != 0 {
		strs = append(strs, "prefers-local")
	}
	if t&DockerDaemonTypeNixpacks != 0 {
		strs = append(strs, "nix-packs")
	}
	if t&DockerDaemonTypeDepot != 0 {
		strs = append(strs, "depot")
	}
	if len(strs) == 0 {
		return "none"
	}

	return strings.Join(strs, ", ")
}

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

func (t DockerDaemonType) UseDepot() bool {
	return (t & DockerDaemonTypeDepot) != 0
}

func (t DockerDaemonType) PrefersLocal() bool {
	return (t & DockerDaemonTypePrefersLocal) != 0
}

func NewLocalDockerClient() (*dockerclient.Client, error) {
	c, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, err
	}

	if _, err = c.Ping(context.TODO()); err != nil {
		return nil, err
	}

	return c, nil
}

func logClearLinesAbove(streams *iostreams.IOStreams, count int) {
	if streams.IsInteractive() {
		builder := aec.EmptyBuilder
		str := builder.Up(uint(count)).EraseLine(aec.EraseModes.All).ANSI
		fmt.Fprint(streams.Out, str.String())
	}
}

func newRemoteDockerClient(ctx context.Context, apiClient flyutil.Client, appName string, streams *iostreams.IOStreams, build *build, cachedClient *dockerclient.Client, connectOverWireguard, recreateBuilder bool) (c *dockerclient.Client, err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "build_remote_docker_client", trace.WithAttributes(
		attribute.Bool("connect_over_wireguard", connectOverWireguard),
	))
	defer span.End()
	if cachedClient != nil {
		span.AddEvent("using cached docker client")
		return cachedClient, nil
	}

	startedAt := time.Now()

	defer func() {
		if err != nil {
			metrics.SendNoData(ctx, "remote_builder_failure")
		}
	}()

	var host string
	var app *fly.App
	var machine *fly.Machine
	machine, app, err = remoteBuilderMachine(ctx, apiClient, appName, recreateBuilder)
	if err != nil {
		tracing.RecordError(span, err, "failed to init remote builder machine")
		return nil, err
	}

	if !connectOverWireguard && !wglessCompatible {
		client := &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return tls.Dial("tcp", fmt.Sprintf("%s.fly.dev:443", app.Name), &tls.Config{})
				},
			},
		}

		url := fmt.Sprintf("http://%s.fly.dev/flyio/v1/settings", app.Name)
		// url := fmt.Sprintf("http://%s.fly.dev/flyio/v1/prune?since='12h'", app.Name)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			tracing.RecordError(span, err, "failed to create remote builder request")
			return nil, err
		}

		req.SetBasicAuth(appName, config.Tokens(ctx).Docker())

		fmt.Fprintln(streams.Out, streams.ColorScheme().Yellow("👀 checking remote builder compatibility with wireguardless deploys ..."))
		span.AddEvent("checking remote builder compatibility with wireguardless deploys")

		res, err := client.Do(req)
		if err != nil {
			tracing.RecordError(span, err, "failed to get remote builder settings")
			return nil, err
		}

		if res.StatusCode == http.StatusNotFound {
			logClearLinesAbove(streams, 1)
			fmt.Fprintln(streams.Out, streams.ColorScheme().Yellow("🔧 automatically deleting and recreating builder"))
			span.AddEvent("automatically deleting and recreating builder")

			err := apiClient.DeleteApp(ctx, app.Name)
			if err != nil {
				tracing.RecordError(span, err, "failed to destroy old incompatible remote builder")
				return nil, err
			}

			fmt.Fprintln(streams.Out, streams.ColorScheme().Yellow("🔧 creating fresh remote builder, (this might take a while ...)"))
			machine, app, err = remoteBuilderMachine(ctx, apiClient, appName, false)
			if err != nil {
				tracing.RecordError(span, err, "failed to init remote builder machine")
				return nil, err
			}
			logClearLinesAbove(streams, 1)
			fmt.Fprintln(streams.Out, streams.ColorScheme().Green("✓ compatible remote builder created"))
		} else {
			logClearLinesAbove(streams, 1)
			fmt.Fprintln(streams.Out, streams.ColorScheme().Green("✓ compatible remote builder found"))
		}

		wglessCompatible = true
	}

	remoteBuilderAppName := app.Name
	remoteBuilderOrg := app.Organization.Slug

	build.SetBuilderMetaPart1(remoteBuilderType, remoteBuilderAppName, machine.ID)

	if msg := fmt.Sprintf("Waiting for remote builder %s...\n", remoteBuilderAppName); streams.IsInteractive() {
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
			sentry.WithTraceID(ctx),
			sentry.WithContexts(map[string]sentry.Context{
				"app": map[string]interface{}{
					"name": appName,
				},
				"organization": map[string]interface{}{
					"name": remoteBuilderOrg,
				},
				"builder": map[string]interface{}{
					"app_name": remoteBuilderAppName,
					"elapsed":  time.Since(startedAt),
				},
			}),
		)
	}

	host = "tcp://[" + machine.PrivateIP + "]:2375"

	if !connectOverWireguard {
		oldHost := host
		host = "https://" + remoteBuilderAppName + ".fly.dev"
		terminal.Infof("Override builder host with: %s (was %s)\n", host, oldHost)

		span.SetAttributes(
			attribute.String("builder.old_host", oldHost),
			attribute.String("builder.host", host),
		)
	}

	span.SetAttributes(
		attribute.String("builder.name", remoteBuilderAppName),
		attribute.String("builder.id", machine.ID),
		attribute.String("builder.host", host),
	)

	if host == "" {
		err = errors.New("machine did not have a private IP")
		tracing.RecordError(span, err, "failed to boot remote builder")
		return nil, err
	}

	builderHostOverride, ok := os.LookupEnv("FLY_RCHAB_OVERRIDE_HOST")
	if ok {
		oldHost := host
		host = builderHostOverride

		span.SetAttributes(
			attribute.String("builder.old_host", oldHost),
			attribute.String("builder.host", host),
		)

		span.AddEvent(fmt.Sprintf("Override builder host with: %s (was %s)\n", host, oldHost))
		terminal.Infof("Override builder host with: %s (was %s)\n", host, oldHost)
	}

	if connectOverWireguard {
		wireguardOpts, err := buildRemoteClientOpts(ctx, apiClient, appName, host)
		if err != nil {
			streams.StopProgressIndicator()
			err = fmt.Errorf("failed building options: %w", err)
			captureError(err)

			if strings.Contains(err.Error(), "timed out") || strings.Contains(err.Error(), "websocket") {
				return nil, generateBrokenWGError(err)
			}

			return nil, err
		}

		wireguardHttpClient, err := dockerclient.NewClientWithOpts(wireguardOpts...)
		if err != nil {
			streams.StopProgressIndicator()

			err = fmt.Errorf("failed creating docker client: %w", err)
			captureError(err)
			tracing.RecordError(span, err, "failed to initialize remote client")

			return nil, err
		}

		cachedClient = wireguardHttpClient
	} else {
		wglessOpts, err := buildWireguardlessClientOpts(ctx, host, appName)
		if err != nil {
			streams.StopProgressIndicator()

			err = fmt.Errorf("failed building wgless options: %w", err)
			captureError(err)
			return nil, err
		}

		wireguardlessHttpsClient, err := dockerclient.NewClientWithOpts(wglessOpts...)
		if err != nil {
			streams.StopProgressIndicator()

			err = fmt.Errorf("failed creating wgLessHttpClient: %w", err)
			captureError(err)
			tracing.RecordError(span, err, "failed to initialize wgLessHttpClient")

			return nil, err
		}
		cachedClient = wireguardlessHttpsClient
	}

	switch up, err := waitForDaemon(ctx, cachedClient); {
	case err != nil:
		streams.StopProgressIndicator()

		err = fmt.Errorf("failed waiting for docker daemon: %w", err)
		captureError(err)
		tracing.RecordError(span, err, "failed to wait for docker daemon")

		if errors.Is(err, agent.ErrTunnelUnavailable) {
			return nil, generateBrokenWGError(err)
		}

		return nil, err
	case !up:
		streams.StopProgressIndicator()
		err := errors.New("remote builder app unavailable")

		terminal.Warnf("Remote builder did not start in time. Check remote builder logs with `flyctl logs -a %s`\n", remoteBuilderAppName)
		tracing.RecordError(span, err, "remote builder failed to start")

		return nil, err
	default:
		if msg := fmt.Sprintf("Remote builder %s ready", remoteBuilderAppName); streams.IsInteractive() {
			streams.StopProgressIndicatorMsg(msg)
		} else {
			fmt.Fprintln(streams.ErrOut, msg)
		}
	}

	return cachedClient, nil
}

func generateBrokenWGError(err error) flyerr.GenericErr {
	return flyerr.GenericErr{
		Err:     err.Error(),
		Suggest: "A broken wireguard tunnel is disrupting your deployment. Retry your deployment with `fly deploy --wg=false` to bypass wireguard ",
	}
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func buildWireguardlessClientOpts(ctx context.Context, host, appName string) ([]dockerclient.Opt, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "build_wgless_client_ops")
	defer span.End()

	parsedHostUrl, err := dockerclient.ParseHostURL(host)
	if err != nil {
		return []dockerclient.Opt{}, fmt.Errorf("failed to parse host: %w", err)
	}

	opts := []dockerclient.Opt{
		dockerclient.WithAPIVersionNegotiation(),
		dockerclient.WithHTTPHeaders(map[string]string{
			"Authorization": "Basic " + basicAuth(appName, config.Tokens(ctx).Docker()),
		}),
		dockerclient.WithDialContext(func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tls.Dial("tcp", parsedHostUrl.Host+":443", &tls.Config{})
		}),
	}

	return opts, nil
}

func buildRemoteClientOpts(ctx context.Context, apiClient flyutil.Client, appName, host string) (opts []dockerclient.Opt, err error) {
	ctx, span := tracing.GetTracer().Start(ctx, "build_remote_client_ops")
	defer span.End()

	opts = []dockerclient.Opt{
		dockerclient.WithAPIVersionNegotiation(),
		dockerclient.WithHost(host),
	}

	if os.Getenv("FLY_REMOTE_BUILDER_HOST_WG") != "" {
		terminal.Debug("connecting to remote docker daemon over host wireguard tunnel")

		return
	}

	url, err := dockerclient.ParseHostURL(host)
	if err != nil {
		tracing.RecordError(span, err, "failed to parse remote builder host")
		return nil, fmt.Errorf("failed to parse remote builder host: %w", err)
	}
	transport := new(http.Transport)
	sockets.ConfigureTransport(transport, url.Scheme, url.Host)
	// Do not try to run tunneled connections through proxy
	transport.Proxy = nil
	opts = append(opts, dockerclient.WithHTTPClient(&http.Client{
		Transport:     transport,
		CheckRedirect: dockerclient.CheckRedirect,
	}))

	var app *fly.AppBasic
	if app, err = apiClient.GetAppBasic(ctx, appName); err != nil {
		tracing.RecordError(span, err, "error fetching target app")
		return nil, fmt.Errorf("error fetching target app: %w", err)
	}

	var agentclient *agent.Client
	if agentclient, err = agent.Establish(ctx, apiClient); err != nil {
		tracing.RecordError(span, err, "failed to establish agent")
		return
	}

	var dialer agent.Dialer
	if dialer, err = agentclient.Dialer(ctx, app.Organization.Slug, ""); err != nil {
		tracing.RecordError(span, err, "failed to dial wg agent")
		return
	}

	if err = agentclient.WaitForTunnel(ctx, app.Organization.Slug, ""); err == nil {
		opts = append(opts, dockerclient.WithDialContext(dialer.DialContext))
	}

	if err != nil {
		tracing.RecordError(span, err, "failed to wait for tunnel")
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
		brokenTunnelErrors   int
		healthyStart         time.Time
	)

	for ctx.Err() == nil {
		switch _, err := clientPing(parent, client); err {
		default:
			if errors.Is(err, agent.ErrTunnelUnavailable) {
				brokenTunnelErrors++
			}

			if brokenTunnelErrors >= 7 {
				return false, err
			}

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

func clientPing(parent context.Context, client *dockerclient.Client) (types.Ping, error) {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()

	return client.Ping(ctx)
}

func clearDeploymentTags(ctx context.Context, docker *dockerclient.Client, tag string) error {
	filters := filters.NewArgs(filters.Arg("reference", tag))

	images, err := docker.ImageList(ctx, image.ListOptions{Filters: filters})
	if err != nil {
		return err
	}

	for _, i := range images {
		for _, tag := range i.RepoTags {
			_, err := docker.ImageRemove(ctx, tag, image.RemoveOptions{PruneChildren: true})
			if err != nil {
				terminal.Debug("Error deleting image", err)
			}
		}
	}

	return nil
}

func registryAuth(token string) registry.AuthConfig {
	targetRegistry := viper.GetString(flyctl.ConfigRegistryHost)
	return registry.AuthConfig{
		Username:      "x",
		Password:      token,
		ServerAddress: targetRegistry,
	}
}

func authConfigs(token string) map[string]registry.AuthConfig {
	targetRegistry := viper.GetString(flyctl.ConfigRegistryHost)

	authConfigs := map[string]registry.AuthConfig{}

	authConfigs[targetRegistry] = registryAuth(token)

	dockerhubUsername := os.Getenv("DOCKER_HUB_USERNAME")
	dockerhubPassword := os.Getenv("DOCKER_HUB_PASSWORD")

	if dockerhubUsername != "" && dockerhubPassword != "" {
		cfg := registry.AuthConfig{
			Username:      dockerhubUsername,
			Password:      dockerhubPassword,
			ServerAddress: "index.docker.io",
		}
		authConfigs["https://index.docker.io/v1/"] = cfg
	}

	return authConfigs
}

func flyRegistryAuth(token string) string {
	authConfig := registryAuth(token)
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
	// MD: this was used by remote builders long ago to set a precomputed ref for deployment.
	// flyd now sets this to the current image in machine env.
	// stop using it in flyctl and if nobody has a problem remove it by 2022-11-01
	// if tag := os.Getenv("FLY_IMAGE_REF"); tag != "" {
	// 	return tag
	// }

	if label == "" {
		label = fmt.Sprintf("deployment-%s", ulid.Make())
	}

	registry := viper.GetString(flyctl.ConfigRegistryHost)

	return fmt.Sprintf("%s/%s:%s", registry, appName, label)
}

func newCacheTag(appName string) string {
	registry := viper.GetString(flyctl.ConfigRegistryHost)

	return fmt.Sprintf("%s/%s:%s", registry, appName, "cache")
}

// ResolveDockerfile - Resolve the location of the dockerfile, allowing for upper and lowercase naming
func ResolveDockerfile(cwd string) string {
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

func EagerlyEnsureRemoteBuilder(ctx context.Context, apiClient flyutil.Client, org *fly.Organization, recreateBuilder bool) {
	// skip if local docker is available
	if _, err := NewLocalDockerClient(); err == nil {
		return
	}

	region := os.Getenv("FLY_REMOTE_BUILDER_REGION")
	_, app, err := EnsureBuilder(ctx, org, region, recreateBuilder)
	if err != nil {
		terminal.Debugf("error ensuring remote builder for organization: %s", err)
		return
	}

	terminal.Debugf("remote builder %s is being prepared", app.Name)
}

func remoteBuilderMachine(ctx context.Context, apiClient flyutil.Client, appName string, recreateBuilder bool) (*fly.Machine, *fly.App, error) {
	if v := os.Getenv("FLY_REMOTE_BUILDER_HOST"); v != "" {
		return nil, nil, nil
	}

	region := os.Getenv("FLY_REMOTE_BUILDER_REGION")
	org, err := apiClient.GetOrganizationByApp(ctx, appName)
	if err != nil {
		return nil, nil, err
	}
	builderMachine, builderApp, err := EnsureBuilder(ctx, org, region, recreateBuilder)
	return builderMachine, builderApp, err
}

func (d *dockerClientFactory) IsRemote() bool {
	return d.remote
}

func (d *dockerClientFactory) IsLocal() bool {
	return !d.remote
}
