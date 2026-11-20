package deployments

import (
	"bytes"
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flag"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/iostreams"
)

// fakeClient returns canned deployments; unset methods panic.
type fakeClient struct {
	client
	getDeployment func(id string, wait int) (*uiex.Deployment, error)
	create        func(in uiex.CreateDeploymentRequest) (*uiex.Deployment, error)
}

func (f *fakeClient) GetDeployment(_ context.Context, _ string, id string, wait int) (*uiex.Deployment, error) {
	return f.getDeployment(id, wait)
}

func (f *fakeClient) CreateDeployment(_ context.Context, _ string, in uiex.CreateDeploymentRequest) (*uiex.Deployment, error) {
	return f.create(in)
}

func ptr(s string) *string { return &s }

func testContext(t *testing.T, c client, follow bool) (ctx context.Context, out, errOut *bytes.Buffer) {
	t.Helper()

	io, _, out, errOut := iostreams.Test()
	ctx = iostreams.NewContext(context.Background(), io)
	ctx = config.NewContext(ctx, &config.Config{})
	ctx = appconfig.WithName(ctx, "my-app")

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Bool("follow", follow, "")
	flags.String("ref", "", "")
	flags.Bool("replace", false, "")
	ctx = flag.NewContext(ctx, flags)

	previous := clientFromContext
	clientFromContext = func(context.Context) (client, error) { return c, nil }
	t.Cleanup(func() { clientFromContext = previous })

	return ctx, out, errOut
}

func TestRedeployFollowsUntilFinished(t *testing.T) {
	var waits []int
	responses := []*uiex.Deployment{
		{ID: 5, Status: "running", CurrentStep: ptr("build")},
		{ID: 5, Status: "complete", Release: &uiex.DeploymentRelease{Version: 12}},
	}

	c := &fakeClient{
		create: func(in uiex.CreateDeploymentRequest) (*uiex.Deployment, error) {
			return &uiex.Deployment{ID: 5, App: "my-app", Status: "running", CurrentStep: ptr("git_pull")}, nil
		},
		getDeployment: func(id string, wait int) (*uiex.Deployment, error) {
			assert.Equal(t, "5", id)
			waits = append(waits, wait)
			next := responses[0]
			responses = responses[1:]
			return next, nil
		},
	}

	ctx, out, errOut := testContext(t, c, true)
	require.NoError(t, runRedeploy(ctx))

	assert.Equal(t, []int{30, 30}, waits)
	assert.Contains(t, errOut.String(), "deployment 5: running: git_pull")
	assert.Contains(t, errOut.String(), "deployment 5: running: build")
	assert.Contains(t, errOut.String(), "deployment 5: complete")
	assert.Contains(t, out.String(), "Release: v12")
}

func TestShowSuggestsDiagnoseOnFailure(t *testing.T) {
	c := &fakeClient{
		getDeployment: func(id string, wait int) (*uiex.Deployment, error) {
			assert.Equal(t, "latest", id)
			return &uiex.Deployment{
				ID:      7,
				App:     "my-app",
				Status:  "failed",
				Failure: &uiex.DeploymentFailure{Step: ptr("build"), Category: "npm_build_error", Title: "npm run build failed"},
				Steps:   []uiex.DeploymentStep{{Name: "build", Description: "Build image", Status: "failed"}},
				Errors:  []uiex.DeploymentError{{Step: "build", Message: "npm run build failed"}},
			}, nil
		},
	}

	ctx, out, _ := testContext(t, c, false)
	require.NoError(t, runShow(ctx))

	assert.Contains(t, out.String(), "Deployment 7 of my-app: failed (npm run build failed)")
	assert.Contains(t, out.String(), "[build] npm run build failed")
	assert.Contains(t, out.String(), "fly deployments diagnose 7")
}
