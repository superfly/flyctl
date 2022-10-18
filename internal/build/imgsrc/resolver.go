package imgsrc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"

	dockerclient "github.com/docker/docker/client"
	"github.com/superfly/flyctl/client"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/iostreams"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/terminal"
)

type ImageOptions struct {
	AppName         string
	WorkingDir      string
	DockerfilePath  string
	ImageRef        string
	BuildArgs       map[string]string
	ExtraBuildArgs  map[string]string
	BuildSecrets    map[string]string
	ImageLabel      string
	Publish         bool
	Tag             string
	Target          string
	NoCache         bool
	BuiltIn         string
	BuiltInSettings map[string]interface{}
	Builder         string
	Buildpacks      []string
}

type RefOptions struct {
	AppName    string
	WorkingDir string
	ImageRef   string
	ImageLabel string
	Publish    bool
	Tag        string
}

type DeploymentImage struct {
	ID   string
	Tag  string
	Size int64
}

type Resolver struct {
	dockerFactory *dockerClientFactory
	apiClient     *api.Client
}

// limit stored logs to 4KB; take suffix if longer
const logLimit int = 4096

// ResolveReference returns an Image give an reference using either the local docker daemon or remote registry
func (r *Resolver) ResolveReference(ctx context.Context, streams *iostreams.IOStreams, opts RefOptions) (img *DeploymentImage, err error) {
	strategies := []imageResolver{
		&localImageResolver{},
		&remoteImageResolver{flyApi: r.apiClient},
	}

	bld, err := r.createImageBuild(ctx, strategies, opts)
	if err != nil {
		terminal.Warnf("failed to create build in graphql: %v\n", err)
	}

	for _, s := range strategies {
		terminal.Debugf("Trying '%s' strategy\n", s.Name())
		bld.ResetTimings()
		bld.BuildAndPushStart()
		var note string
		img, note, err = s.Run(ctx, r.dockerFactory, streams, opts, bld)
		terminal.Debugf("result image:%+v error:%v\n", img, err)
		if err != nil {
			bld.BuildAndPushFinish()
			bld.FinishImageStrategy(s, true /* failed */, err, note)
			r.finishBuild(ctx, bld, true /* failed */, err.Error(), nil)
			return nil, err
		}
		if img != nil {
			bld.BuildAndPushFinish()
			bld.FinishImageStrategy(s, false /* success */, nil, note)
			r.finishBuild(ctx, bld, false /* completed */, "", img)
			return img, nil
		}
		bld.BuildAndPushFinish()
		bld.FinishImageStrategy(s, true /* failed */, nil, note)
	}

	r.finishBuild(ctx, bld, true /* failed */, "no strategies resulted in an image", nil)
	return nil, fmt.Errorf("could not find image \"%s\"", opts.ImageRef)
}

// BuildImage converts source code to an image using a Dockerfile, buildpacks, or builtins.
func (r *Resolver) BuildImage(ctx context.Context, streams *iostreams.IOStreams, opts ImageOptions) (img *DeploymentImage, err error) {
	if !r.dockerFactory.mode.IsAvailable() {
		return nil, errors.New("docker is unavailable to build the deployment image")
	}

	if opts.Tag == "" {
		opts.Tag = NewDeploymentTag(opts.AppName, opts.ImageLabel)
	}

	strategies := []imageBuilder{}

	if r.dockerFactory.mode.UseNixpacks() {
		strategies = append(strategies, &nixpacksBuilder{})
	} else {
		strategies = []imageBuilder{
			&buildpacksBuilder{},
			&dockerfileBuilder{},
			&builtinBuilder{},
		}
	}
	bld, err := r.createBuild(ctx, strategies, opts)
	if err != nil {
		terminal.Warnf("failed to create build in graphql: %v\n", err)
	}
	for _, s := range strategies {
		terminal.Debugf("Trying '%s' strategy\n", s.Name())
		bld.ResetTimings()
		bld.BuildAndPushStart()
		var note string
		img, note, err = s.Run(ctx, r.dockerFactory, streams, opts, bld)
		terminal.Debugf("result image:%+v error:%v\n", img, err)
		if err != nil {
			bld.BuildAndPushFinish()
			bld.FinishStrategy(s, true /* failed */, err, note)
			r.finishBuild(ctx, bld, true /* failed */, err.Error(), nil)
			return nil, err
		}
		if img != nil {
			bld.BuildAndPushFinish()
			bld.FinishStrategy(s, false /* success */, nil, note)
			r.finishBuild(ctx, bld, false /* completed */, "", img)
			return img, nil
		}
		bld.BuildAndPushFinish()
		bld.FinishStrategy(s, true /* failed */, nil, note)
	}

	r.finishBuild(ctx, bld, true /* failed */, "no strategies resulted in an image", nil)
	return nil, errors.New("app does not have a Dockerfile or buildpacks configured. See https://fly.io/docs/reference/configuration/#the-build-section")
}

func (r *Resolver) createImageBuild(ctx context.Context, strategies []imageResolver, opts RefOptions) (*build, error) {
	strategiesAvailable := make([]string, 0)
	for _, r := range strategies {
		strategiesAvailable = append(strategiesAvailable, r.Name())
	}
	imageOps := &gql.BuildImageOptsInput{
		ImageLabel: opts.ImageLabel,
		ImageRef:   opts.ImageRef,
		Publish:    opts.Publish,
		Tag:        opts.Tag,
	}
	return r.createBuildGql(ctx, strategiesAvailable, imageOps)
}

func (r *Resolver) createBuild(ctx context.Context, strategies []imageBuilder, opts ImageOptions) (*build, error) {

	strategiesAvailable := make([]string, 0)
	for _, s := range strategies {
		strategiesAvailable = append(strategiesAvailable, s.Name())
	}
	imageOpts := &gql.BuildImageOptsInput{
		BuildArgs:       opts.BuildArgs,
		BuildPacks:      opts.Buildpacks,
		Builder:         opts.Builder,
		BuiltIn:         opts.BuiltIn,
		BuiltInSettings: opts.BuiltInSettings,
		DockerfilePath:  opts.DockerfilePath,
		ExtraBuildArgs:  opts.ExtraBuildArgs,
		ImageLabel:      opts.ImageLabel,
		ImageRef:        opts.ImageRef,
		NoCache:         opts.NoCache,
		Publish:         opts.Publish,
		Tag:             opts.Tag,
		Target:          opts.Target,
	}
	return r.createBuildGql(ctx, strategiesAvailable, imageOpts)
}

func (r *Resolver) createBuildGql(ctx context.Context, strategiesAvailable []string, imageOpts *gql.BuildImageOptsInput) (*build, error) {
	gqlClient := client.FromContext(ctx).API().GenqClient
	_ = `# @genqlient
	mutation ResolverCreateBuild($input:CreateBuildInput!) {
		createBuild(input:$input) {
			id
			status
		}
	}
	`
	builderType := "local"
	if r.dockerFactory.remote {
		builderType = "remote"
	}
	input := gql.CreateBuildInput{
		AppName:             r.dockerFactory.appName,
		BuilderType:         builderType,
		ImageOpts:           *imageOpts,
		MachineId:           "",
		StrategiesAvailable: strategiesAvailable,
	}
	resp, err := gql.ResolverCreateBuild(ctx, gqlClient, input)
	if err != nil {
		sentry.CaptureException(err,
			sentry.WithTag("feature", "build-api-create-build"),
			sentry.WithContexts(map[string]interface{}{
				"app": map[string]interface{}{
					"name": input.AppName,
				},
				"builder": map[string]interface{}{
					"type": input.BuilderType,
				},
			}),
		)
		return newFailedBuild(), err
	}

	b := newBuild(resp.CreateBuild.Id, false)
	// TODO: maybe try to capture SIGINT, SIGTERM and send r.FinishBuild(). I tried, and it usually segfaulted. (tvd, 2022-10-14)
	return b, nil
}

func limitLogs(log string) string {
	if len(log) > logLimit {
		return log[len(log)-logLimit:]
	}
	return log
}

type build struct {
	CreateApiFailed bool
	BuildId         string
	BuilderMeta     *gql.BuilderMetaInput
	StrategyResults []gql.BuildStrategyAttemptInput
	Timings         *gql.BuildTimingsInput
	StartTimes      *gql.BuildTimingsInput
}

func newFailedBuild() *build {
	return newBuild("", true)
}

func newBuild(buildId string, createApiFailed bool) *build {
	return &build{
		CreateApiFailed: createApiFailed,
		BuildId:         buildId,
		BuilderMeta:     &gql.BuilderMetaInput{},
		StrategyResults: make([]gql.BuildStrategyAttemptInput, 0),
		StartTimes:      &gql.BuildTimingsInput{},
		Timings: &gql.BuildTimingsInput{
			BuildAndPushMs: -1,
			BuilderInitMs:  -1,
			BuildMs:        -1,
			ContextBuildMs: -1,
			ImageBuildMs:   -1,
			PushMs:         -1,
		},
	}
}

func (b *build) SetBuilderMetaPart1(remote bool, remoteAppName string, remoteMachineId string) {
	if b == nil {
		return
	}
	builderType := "remote"
	if !remote {
		builderType = "local"
	}
	b.BuilderMeta.BuilderType = builderType
	b.BuilderMeta.RemoteAppName = remoteAppName
	b.BuilderMeta.RemoteMachineId = remoteMachineId
}
func (b *build) SetBuilderMetaPart2(buildkitEnabled bool, dockerVersion string, platform string) {
	b.BuilderMeta.BuildkitEnabled = buildkitEnabled
	b.BuilderMeta.DockerVersion = dockerVersion
	b.BuilderMeta.Platform = platform
}

// call this at the start of each strategy to restart all the timers
func (b *build) ResetTimings() {
	b.StartTimes = &gql.BuildTimingsInput{}
	b.Timings = &gql.BuildTimingsInput{
		BuildAndPushMs: -1,
		BuilderInitMs:  -1,
		BuildMs:        -1,
		ContextBuildMs: -1,
		ImageBuildMs:   -1,
		PushMs:         -1,
	}
}

func (b *build) BuildAndPushStart() {
	b.StartTimes.BuildAndPushMs = time.Now().UnixMilli()
}
func (b *build) BuildAndPushFinish() {
	b.Timings.BuildAndPushMs = time.Now().UnixMilli() - b.StartTimes.BuildAndPushMs
}
func (b *build) BuilderInitStart() {
	b.StartTimes.BuilderInitMs = time.Now().UnixMilli()
}
func (b *build) BuilderInitFinish() {
	b.Timings.BuilderInitMs = time.Now().UnixMilli() - b.StartTimes.BuilderInitMs
}
func (b *build) BuildStart() {
	b.StartTimes.BuildMs = time.Now().UnixMilli()
}
func (b *build) BuildFinish() {
	b.Timings.BuildMs = time.Now().UnixMilli() - b.StartTimes.BuildMs
}
func (b *build) ContextBuildStart() {
	b.StartTimes.ContextBuildMs = time.Now().UnixMilli()
}
func (b *build) ContextBuildFinish() {
	b.Timings.ContextBuildMs = time.Now().UnixMilli() - b.StartTimes.ContextBuildMs
}
func (b *build) ImageBuildStart() {
	b.StartTimes.ImageBuildMs = time.Now().UnixMilli()
}
func (b *build) ImageBuildFinish() {
	b.Timings.ImageBuildMs = time.Now().UnixMilli() - b.StartTimes.ImageBuildMs
}
func (b *build) PushStart() {
	b.StartTimes.PushMs = time.Now().UnixMilli()
}
func (b *build) PushFinish() {
	b.Timings.PushMs = time.Now().UnixMilli() - b.StartTimes.PushMs
}

func (b *build) finishStrategyCommon(strategy string, failed bool, err error, note string) {
	result := "failed"
	if !failed {
		result = "success"
	}
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	r := gql.BuildStrategyAttemptInput{
		Strategy: strategy,
		Result:   result,
		Error:    limitLogs(errMsg),
		Note:     limitLogs(note),
	}
	b.StrategyResults = append(b.StrategyResults, r)
}

func (b *build) FinishStrategy(strategy imageBuilder, failed bool, err error, note string) {
	b.finishStrategyCommon(strategy.Name(), failed, err, note)
}

func (b *build) FinishImageStrategy(strategy imageResolver, failed bool, err error, note string) {
	b.finishStrategyCommon(strategy.Name(), failed, err, note)
}

type buildResult struct {
	BuildId         string
	Status          string
	wallclockTimeMs int
}

func (r *Resolver) finishBuild(ctx context.Context, build *build, failed bool, logs string, img *DeploymentImage) (*buildResult, error) {
	if build.CreateApiFailed {
		terminal.Debug("Skipping FinishBuild() gql call, because CreateBuild() failed.\n")
		return nil, nil
	}
	gqlClient := client.FromContext(ctx).API().GenqClient
	_ = `# @genqlient
	mutation ResolverFinishBuild($input:FinishBuildInput!) {
		finishBuild(input:$input) {
			id
			status
			wallclockTimeMs
		}
	}
	`
	status := "failed"
	if !failed {
		status = "completed"
	}
	input := gql.FinishBuildInput{
		BuildId:             build.BuildId,
		AppName:             r.dockerFactory.appName,
		MachineId:           "",
		Status:              status,
		Logs:                limitLogs(logs),
		BuilderMeta:         *build.BuilderMeta,
		StrategiesAttempted: build.StrategyResults,
		Timings:             *build.Timings,
	}
	if img != nil {
		input.FinalImage = gql.BuildFinalImageInput{
			Id:        img.ID,
			Tag:       img.Tag,
			SizeBytes: img.Size,
		}
	}
	resp, err := gql.ResolverFinishBuild(ctx, gqlClient, input)
	if err != nil {
		terminal.Warnf("failed to finish build in graphql: %v\n", err)
		sentry.CaptureException(err,
			sentry.WithTag("feature", "build-api-finish-build"),
			sentry.WithContexts(map[string]interface{}{
				"app": map[string]interface{}{
					"name": r.dockerFactory.appName,
				},
				"sourceBuild": map[string]interface{}{
					"id": build.BuildId,
				},
				"builder": map[string]interface{}{
					"type":            build.BuilderMeta.BuilderType,
					"appName":         build.BuilderMeta.RemoteAppName,
					"machineId":       build.BuilderMeta.RemoteMachineId,
					"dockerVersion":   build.BuilderMeta.DockerVersion,
					"buildkitEnabled": build.BuilderMeta.BuildkitEnabled,
				},
			}),
		)
		return nil, err
	}
	return &buildResult{
		BuildId:         resp.FinishBuild.Id,
		Status:          resp.FinishBuild.Status,
		wallclockTimeMs: resp.FinishBuild.WallclockTimeMs,
	}, nil
}

// For remote builders send a periodic heartbeat during build to ensure machine stays alive
// This is a noop for local builders
func (r *Resolver) StartHeartbeat(ctx context.Context) chan<- interface{} {
	if !r.dockerFactory.remote {
		return nil
	}

	errMsg := "Failed to start remote builder heartbeat: %v\n"
	dockerClient, err := r.dockerFactory.buildFn(ctx, nil)
	if err != nil {
		terminal.Warnf(errMsg, err)
		return nil
	}
	heartbeatUrl, err := getHeartbeatUrl(dockerClient)
	if err != nil {
		terminal.Warnf(errMsg, err)
		return nil
	}
	heartbeatReq, err := http.NewRequestWithContext(ctx, http.MethodGet, heartbeatUrl, http.NoBody)
	if err != nil {
		terminal.Warnf(errMsg, err)
		return nil
	}
	heartbeatReq.SetBasicAuth(r.dockerFactory.appName, flyctl.GetAPIToken())
	heartbeatReq.Header.Set("User-Agent", fmt.Sprintf("flyctl/%s", buildinfo.Version().String()))

	terminal.Debugf("Sending remote builder heartbeat pulse to %s...\n", heartbeatUrl)
	resp, err := dockerClient.HTTPClient().Do(heartbeatReq)
	if err != nil {
		terminal.Debugf("Remote builder heartbeat pulse failed, not going to run heartbeat: %v\n", err)
		return nil
	} else if resp.StatusCode != http.StatusAccepted {
		terminal.Debugf("Unexpected remote builder heartbeat response, not going to run heartbeat: %s\n", resp.Status)
		return nil
	}

	pulseInterval := 30 * time.Second
	maxTime := 1 * time.Hour

	done := make(chan interface{})
	time.AfterFunc(maxTime, func() { close(done) })

	go func() {
		pulse := time.NewTicker(pulseInterval)
		defer close(done)
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-pulse.C:
				terminal.Debugf("Sending remote builder heartbeat pulse to %s...\n", heartbeatUrl)
				resp, err := dockerClient.HTTPClient().Do(heartbeatReq)
				if err != nil {
					terminal.Debugf("Remote builder heartbeat pulse failed: %v\n", err)
				} else {
					terminal.Debugf("Remote builder heartbeat response: %s\n", resp.Status)
				}
			}
		}
	}()
	return done
}

func getHeartbeatUrl(dockerClient *dockerclient.Client) (string, error) {
	daemonHost := dockerClient.DaemonHost()
	parsed, err := url.Parse(daemonHost)
	if err != nil {
		return "", err
	}
	hostPort := parsed.Host
	host, _, _ := net.SplitHostPort(hostPort)
	parsed.Host = host + ":8080"
	parsed.Scheme = "http"
	parsed.Path = "/flyio/v1/extendDeadline"
	return parsed.String(), nil
}

func (r *Resolver) StopHeartbeat(heartbeat chan<- interface{}) {
	if heartbeat != nil {
		heartbeat <- struct{}{}
	}
}

func NewResolver(daemonType DockerDaemonType, apiClient *api.Client, appName string, iostreams *iostreams.IOStreams) *Resolver {
	return &Resolver{
		dockerFactory: newDockerClientFactory(daemonType, apiClient, appName, iostreams),
		apiClient:     apiClient,
	}
}

type imageBuilder interface {
	Name() string
	Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error)
}

type imageResolver interface {
	Name() string
	Run(ctx context.Context, dockerFactory *dockerClientFactory, streams *iostreams.IOStreams, opts RefOptions, build *build) (*DeploymentImage, string, error)
}
