package flypg

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/iostreams"
)

// errStopLaunch ends LaunchMachinesPostgres right after the Flycast address is
// requested, so the test only exercises app creation and IP allocation.
var errStopLaunch = errors.New("stop after AssignIP")

// launchUntilAssignIP runs LaunchMachinesPostgres for a Flex cluster and
// returns the CreateApp and AssignIP requests it sent.
func launchUntilAssignIP(t *testing.T, network string) (flaps.CreateAppRequest, flaps.AssignIPRequest, int) {
	t.Helper()

	var (
		createReq   flaps.CreateAppRequest
		assignReq   flaps.AssignIPRequest
		assignCalls int
	)

	flapsClient := &mock.FlapsClient{
		CreateAppFunc: func(_ context.Context, req flaps.CreateAppRequest) (*flaps.App, error) {
			createReq = req
			return &flaps.App{ID: "app-id", Name: req.Name}, nil
		},
		WaitForAppFunc: func(context.Context, string) error { return nil },
		AssignIPFunc: func(_ context.Context, appName string, req flaps.AssignIPRequest) (*flaps.AssignIPResponse, error) {
			assignCalls++
			assert.Equal(t, "my-pg", appName)
			assignReq = req
			return nil, errStopLaunch
		},
	}

	ios, _, _, _ := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	err := NewLauncher(nil).LaunchMachinesPostgres(ctx, &CreateClusterInput{
		AppName:      "my-pg",
		Network:      network,
		Organization: &uiex.Organization{ID: "org-id", Slug: "my-org"},
		// A preset image skips the latest-image lookup on the API client.
		ImageRef: "flyio/postgres-flex:17",
		Region:   "iad",
		Manager:  ReplicationManager,
		Guest:    &fly.MachineGuest{CPUKind: "shared", CPUs: 1, MemoryMB: 1024},
	}, true)
	require.ErrorIs(t, err, errStopLaunch)

	return createReq, assignReq, assignCalls
}

func TestLaunchMachinesPostgresCustomNetwork(t *testing.T) {
	createReq, assignReq, assignCalls := launchUntilAssignIP(t, "pgnet")

	assert.Equal(t, flaps.CreateAppRequest{
		Org:       "my-org",
		Name:      "my-pg",
		Network:   "pgnet",
		AppRoleID: "postgres_cluster",
	}, createReq)

	// The Flycast address must be on the same network as the app, or the
	// cluster is reachable from the default network and not from its peers.
	require.Equal(t, 1, assignCalls)
	assert.Equal(t, flaps.AssignIPRequest{
		Type:    flaps.IPAssignmentTypePrivateV6,
		Network: "pgnet",
	}, assignReq)
}

func TestLaunchMachinesPostgresDefaultNetwork(t *testing.T) {
	createReq, assignReq, _ := launchUntilAssignIP(t, "")

	// Without --network both requests are the same as before the flag existed.
	assert.Empty(t, createReq.Network)
	assert.Equal(t, flaps.AssignIPRequest{Type: flaps.IPAssignmentTypePrivateV6}, assignReq)
}

func TestConnectHint(t *testing.T) {
	org := &uiex.Organization{Name: "Acme"}

	t.Run("default network", func(t *testing.T) {
		assert.Equal(t,
			"Any app within the Acme organization can connect to this Postgres using the above connection string",
			connectHint(&CreateClusterInput{Organization: org}))
	})

	t.Run("custom network", func(t *testing.T) {
		got := connectHint(&CreateClusterInput{Organization: org, Network: "pgnet"})
		assert.Contains(t, got, "Apps on the pgnet network in the Acme organization can connect")
		assert.Contains(t, got, "can't reach it")
		assert.NotContains(t, got, "Any app within")
	})
}
