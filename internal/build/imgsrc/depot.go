package imgsrc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cenkalti/backoff/v5"
	depotbuild "github.com/depot/depot-go/build"
	depotmachine "github.com/depot/depot-go/machine"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/pkg/errors"
	"github.com/superfly/fly-go"
	"github.com/superfly/flyctl/helpers"
	"github.com/superfly/flyctl/internal/cmdfmt"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/render"
	"github.com/superfly/flyctl/internal/tracing"
	"github.com/superfly/flyctl/iostreams"
	"github.com/superfly/flyctl/terminal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"
)

var _ imageBuilder = (*DepotBuilder)(nil)

type depotBuilderScope int

const (
	DepotBuilderScopeOrganization depotBuilderScope = iota
	DepotBuilderScopeApp
)

func (s depotBuilderScope) String() string {
	switch s {
	case DepotBuilderScopeOrganization:
		return "organization"
	case DepotBuilderScopeApp:
		return "app"
	default:
		return "unknown"
	}
}

type DepotBuilder struct {
	Scope depotBuilderScope
}

func (d *DepotBuilder) Name() string { return "depot.dev" }

func (d *DepotBuilder) Run(ctx context.Context, _ *dockerClientFactory, streams *iostreams.IOStreams, opts ImageOptions, build *build) (*DeploymentImage, string, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "depot_builder", trace.WithAttributes(opts.ToSpanAttributes()...))
	defer span.End()

	build.BuildStart()

	var dockerfile string

	switch {
	case opts.DockerfilePath != "" && !helpers.FileExists(opts.DockerfilePath):
		build.BuildFinish()
		err := fmt.Errorf("dockerfile '%s' not found", opts.DockerfilePath)
		tracing.RecordError(span, err, "failed to find dockerfile")
		return nil, "", err
	case opts.DockerfilePath != "":
		dockerfile = opts.DockerfilePath
	default:
		dockerfile = ResolveDockerfile(opts.WorkingDir)
	}

	if dockerfile == "" {
		span.AddEvent("dockerfile not found, skipping")
		terminal.Debug("dockerfile not found, skipping")
		build.BuildFinish()
		return nil, "", nil
	}

	var relDockerfile string
	if isPathInRoot(dockerfile, opts.WorkingDir) {
		// pass the relative path to Dockerfile within the context
		p, err := filepath.Rel(opts.WorkingDir, dockerfile)
		if err != nil {
			tracing.RecordError(span, err, "failed to get relative dockerfile path")
			build.BuildFinish()
			return nil, "", err
		}
		// On Windows, convert \ to a slash / as the docker build will
		// run in a Linux VM at the end.
		relDockerfile = filepath.ToSlash(p)
	}
	span.SetAttributes(attribute.String("relative_dockerfile_path", relDockerfile))

	build.ImageBuildStart()

	image, err := depotBuild(ctx, streams, opts, dockerfile, build, d.Scope)
	if err != nil {
		metrics.SendNoData(ctx, "remote_builder_failure")
		build.ImageBuildFinish()
		build.BuildFinish()
		tracing.RecordError(span, err, "failed to build image")
		return nil, "", errors.Wrap(err, "error building")
	}

	build.ImageBuildFinish()
	build.BuildFinish()
	cmdfmt.PrintDone(streams.ErrOut, "Building image done")

	span.SetAttributes(image.ToSpanAttributes()...)
	return image, "", nil
}

func depotBuild(ctx context.Context, streams *iostreams.IOStreams, opts ImageOptions, dockerfilePath string, buildState *build, scope depotBuilderScope) (*DeploymentImage, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "depot_build", trace.WithAttributes(opts.ToSpanAttributes()...))
	defer span.End()

	buildState.BuilderInitStart()
	buildState.SetBuilderMetaPart1(depotBuilderType, "", "")

	{
		msg := "Waiting for depot builder...\n"
		if streams.IsInteractive() {
			streams.StartProgressIndicatorMsg(msg)
		} else {
			fmt.Fprintln(streams.ErrOut, msg)
		}
	}

	buildkit, build, buildErr := initBuilder(ctx, buildState, opts.AppName, streams, scope)
	if buildErr != nil {
		streams.StopProgressIndicator()
		span.RecordError(buildErr)
		return nil, buildErr
	}
	defer func() {
		buildkit.Release()
		build.Finish(buildErr)
	}()

	connectCtx, cancelConnect := context.WithTimeout(ctx, 5*time.Minute)
	defer cancelConnect()

	span.AddEvent("connecting to buildkit")
	var buildkitClient *client.Client
	buildkitClient, buildErr = buildkit.Connect(connectCtx)
	if buildErr != nil {
		streams.StopProgressIndicator()
		span.RecordError(buildErr)
		return nil, buildErr
	}

	streams.StopProgressIndicator()

	tb := render.NewTextBlock(ctx, "Building image with Depot")
	link := streams.CreateLink("build: ", build.BuildURL)
	tb.Done(link)

	buildState.BuildAndPushStart()
	res, buildErr := buildImage(ctx, buildkitClient, opts, dockerfilePath)
	if buildErr != nil {
		buildState.BuildAndPushFinish()
		span.RecordError(buildErr)
		return nil, buildErr
	}
	buildState.BuildAndPushFinish()

	link = streams.CreateLink("Build Summary: ", build.BuildURL)
	tb.Done(link)

	return newDeploymentImage(res, opts.Tag)
}

func initBuilder(ctx context.Context, buildState *build, appName string, streams *iostreams.IOStreams, builderScope depotBuilderScope) (*depotmachine.Machine, *depotbuild.Build, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "init_depot_build")
	defer span.End()

	defer buildState.BuilderInitFinish()

	apiClient := flyutil.ClientFromContext(ctx)
	region := os.Getenv("FLY_REMOTE_BUILDER_REGION")
	if region != "" {
		region = "fly-" + region
	}
	span.SetAttributes(attribute.String("depot_builder_region", region))

	buildInfo, err := apiClient.EnsureDepotRemoteBuilder(ctx, &fly.EnsureDepotRemoteBuilderInput{
		AppName:      &appName,
		Region:       &region,
		BuilderScope: fly.StringPointer(builderScope.String()),
	})
	if err != nil {
		streams.StopProgressIndicator()
		return nil, nil, err
	}

	build, err := depotbuild.FromExistingBuild(ctx, *buildInfo.EnsureDepotRemoteBuilder.BuildId, *buildInfo.EnsureDepotRemoteBuilder.BuildToken)
	if err != nil {
		return nil, nil, err
	}

	// Set the buildErr to any error that represents the build failing.
	var finalBuildErr error

	span.AddEvent("Acquiring Depot machine")

	timeoutCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	buildkit, finalBuildErr := backoff.Retry(timeoutCtx, func() (*depotmachine.Machine, error) {
		machine, err := depotmachine.Acquire(ctx, build.ID, build.Token, "amd64")
		if err != nil {
			span.RecordError(err)
			return nil, err
		}

		return machine, nil
	}, backoff.WithMaxTries(2))

	if finalBuildErr != nil {
		streams.StopProgressIndicator()
		return nil, nil, finalBuildErr
	}

	return buildkit, &build, err
}

func buildImage(ctx context.Context, buildkitClient *client.Client, opts ImageOptions, dockerfilePath string) (*client.SolveResponse, error) {
	ctx, span := tracing.GetTracer().Start(ctx, "depot_build_image", trace.WithAttributes(opts.ToSpanAttributes()...))
	defer span.End()

	var (
		res *client.SolveResponse
		err error
	)

	exportEntry := client.ExportEntry{
		Type: "image",
		Attrs: map[string]string{
			"name":           opts.Tag,
			"oci-mediatypes": "true",
		},
	}

	if opts.Publish {
		exportEntry.Attrs["push"] = "true"
	}

	if opts.UseZstd {
		exportEntry.Attrs["compression"] = "zstd"
		exportEntry.Attrs["compression-level"] = "3"
		exportEntry.Attrs["force-compression"] = "true"
	}

	ch := make(chan *client.SolveStatus)
	eg, ctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		solverOptions := client.SolveOpt{
			Frontend: "dockerfile.v0",
			FrontendAttrs: map[string]string{
				"filename": filepath.Base(dockerfilePath),
				"target":   opts.Target,
				"platform": "linux/amd64",
			},
			LocalDirs: map[string]string{
				"dockerfile": filepath.Dir(dockerfilePath),
				"context":    opts.WorkingDir,
			},
			Exports: []client.ExportEntry{exportEntry},
			// Prevent recording the build steps and traces in buildkit as it is _very_ slow.
			Internal: true,
		}
		if opts.NoCache {
			solverOptions.FrontendAttrs["no-cache"] = ""
		}
		for k, v := range opts.Label {
			solverOptions.FrontendAttrs["label:"+k] = v
		}
		for k, v := range opts.BuildArgs {
			solverOptions.FrontendAttrs["build-arg:"+k] = v
		}

		secrets := make(map[string][]byte)
		for k, v := range opts.BuildSecrets {
			secrets[k] = []byte(v)
		}
		solverOptions.Session = append(
			solverOptions.Session,
			newBuildkitAuthProvider(config.Tokens(ctx).Docker()),
			secretsprovider.FromMap(secrets),
		)

		res, err = buildkitClient.Solve(ctx, nil, solverOptions, ch)
		return err
	})

	eg.Go(func() error {
		display, err := progressui.NewDisplay(os.Stderr, progressui.AutoMode)
		if err != nil {
			return err
		}

		_, err = display.UpdateFrom(context.Background(), ch)
		return err
	})

	if err := eg.Wait(); err != nil {
		span.RecordError(err)
		return nil, err
	}

	return res, nil
}

func newDeploymentImage(res *client.SolveResponse, tag string) (*DeploymentImage, error) {
	id := res.ExporterResponse["containerimage.digest"]
	encoded := res.ExporterResponse["containerimage.descriptor"]
	output, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}

	descriptor := &Descriptor{}
	err = json.Unmarshal(output, descriptor)
	if err != nil {
		return nil, err
	}

	image := &DeploymentImage{
		ID:   id,
		Tag:  tag,
		Size: descriptor.Bytes(),
	}

	return image, nil
}

type Descriptor struct {
	MediaType   string      `json:"mediaType,omitempty"`
	Digest      string      `json:"digest,omitempty"`
	Size        int64       `json:"size,omitempty"`
	Annotations Annotations `json:"annotations,omitempty"`
}

func (d *Descriptor) Bytes() int64 {
	return d.Size + d.Annotations.Bytes()
}

type Annotations struct {
	RawManifest string `json:"depot.containerimage.manifest,omitempty"`
}

func (a *Annotations) Manifest() (*Manifest, error) {
	manifest := &Manifest{}
	err := json.Unmarshal([]byte(a.RawManifest), manifest)
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

func (a *Annotations) Bytes() int64 {
	manifest, err := a.Manifest()
	if err != nil {
		log.Printf("failed to get manifest: %v", err)
		return 0
	}
	return manifest.Bytes()
}

type Manifest struct {
	SchemaVersion int             `json:"schemaVersion,omitempty"`
	MediaType     string          `json:"mediaType,omitempty"`
	Config        OCIDescriptor   `json:"config,omitempty"`
	Layers        []OCIDescriptor `json:"layers,omitempty"`
}

func (m *Manifest) Bytes() int64 {
	size := m.Config.Size
	for _, layer := range m.Layers {
		size += layer.Size
	}
	return size
}

type OCIDescriptor struct {
	MediaType string `json:"mediaType,omitempty"`
	Digest    string `json:"digest,omitempty"`
	Size      int64  `json:"size,omitempty"`
}
