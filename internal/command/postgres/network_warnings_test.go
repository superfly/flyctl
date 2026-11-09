package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

// warningContext returns a context with flapsClient and a getter for stderr,
// where both network warnings are written. stdout must stay empty so scripts
// reading command output aren't affected.
func warningContext(t *testing.T, flapsClient *mock.FlapsClient) (context.Context, func() string) {
	t.Helper()

	ios, _, stdout, stderr := iostreams.Test()
	t.Cleanup(func() { assert.Empty(t, stdout.String(), "network warnings must go to stderr") })

	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = flapsutil.NewContextWithClient(ctx, flapsClient)

	return ctx, stderr.String
}

func TestForkNetwork(t *testing.T) {
	source := &flaps.App{Name: "pg-a", Network: "pgnet"}

	assert.Equal(t, "pgnet", forkNetwork("", false, source), "a fork stays on the source's network")
	assert.Equal(t, "other", forkNetwork("other", true, source), "--network overrides the source's network")
	assert.Empty(t, forkNetwork("", true, source), "--network default moves the fork to the default network")
	assert.Empty(t, forkNetwork("", false, &flaps.App{Name: "pg-b", Network: flapsutil.DefaultNetwork}), "a default-network source forks onto the default network")
}

func TestWarnIfNewNetwork(t *testing.T) {
	listing := []flaps.App{
		{Name: "web", Network: "pgnet"},
		{Name: "api", Network: flapsutil.DefaultNetwork},
	}

	t.Run("existing network", func(t *testing.T) {
		ctx, stderr := warningContext(t, &mock.FlapsClient{
			ListAppsFunc: func(_ context.Context, req flaps.ListAppsRequest) ([]flaps.App, error) {
				assert.Equal(t, "my-org", req.OrgSlug)
				return listing, nil
			},
		})

		warnIfNewNetwork(ctx, "my-org", "pgnet")
		assert.Empty(t, stderr())
	})

	t.Run("new network", func(t *testing.T) {
		ctx, stderr := warningContext(t, &mock.FlapsClient{
			ListAppsFunc: func(context.Context, flaps.ListAppsRequest) ([]flaps.App, error) { return listing, nil },
		})

		warnIfNewNetwork(ctx, "my-org", "pgnte")
		assert.Contains(t, stderr(), `No apps in my-org are on network "pgnte" yet, so a new network will be created`)
	})

	t.Run("lookup fails", func(t *testing.T) {
		ctx, stderr := warningContext(t, &mock.FlapsClient{
			ListAppsFunc: func(context.Context, flaps.ListAppsRequest) ([]flaps.App, error) {
				return nil, errors.New("boom")
			},
		})

		// A failed lookup must not block creation or print a false warning.
		warnIfNewNetwork(ctx, "my-org", "pgnet")
		assert.Empty(t, stderr())
	})
}

func flycastOn(network string) flaps.IPAssignment {
	return flaps.IPAssignment{IP: "fdaa:0:1::3", Network: &flaps.IPAssignmentNetwork{Name: network}}
}

func TestWarnIfUnreachable(t *testing.T) {
	pg := &flaps.App{Name: "my-pg", Network: "pgnet"}

	t.Run("same network skips the IP lookup", func(t *testing.T) {
		// No GetIPAssignmentsFunc: calling it would panic.
		ctx, stderr := warningContext(t, &mock.FlapsClient{})

		warnIfUnreachable(ctx, pg, &flaps.App{Name: "web", Network: "pgnet"})
		// "default" and "" both mean the default network.
		warnIfUnreachable(ctx, &flaps.App{Name: "old-pg", Network: flapsutil.DefaultNetwork}, &flaps.App{Name: "web"})
		assert.Empty(t, stderr())
	})

	t.Run("different network without a Flycast address there", func(t *testing.T) {
		ctx, stderr := warningContext(t, &mock.FlapsClient{
			GetIPAssignmentsFunc: func(_ context.Context, appName string) (*flaps.ListIPAssignmentsResponse, error) {
				assert.Equal(t, "my-pg", appName)
				return &flaps.ListIPAssignmentsResponse{IPs: []flaps.IPAssignment{flycastOn("pgnet")}}, nil
			},
		})

		warnIfUnreachable(ctx, pg, &flaps.App{Name: "web", Network: flapsutil.DefaultNetwork})
		out := stderr()
		assert.Contains(t, out, "web is on network default and my-pg is on network pgnet, so web won't be able to reach the database")
		assert.Contains(t, out, "fly ips allocate-v6 --private -a my-pg\n", "the default network is selected by leaving --network off")
		assert.NotContains(t, out, "--network default")
	})

	t.Run("app on another custom network", func(t *testing.T) {
		ctx, stderr := warningContext(t, &mock.FlapsClient{
			GetIPAssignmentsFunc: func(context.Context, string) (*flaps.ListIPAssignmentsResponse, error) {
				return &flaps.ListIPAssignmentsResponse{IPs: []flaps.IPAssignment{flycastOn("pgnet")}}, nil
			},
		})

		warnIfUnreachable(ctx, pg, &flaps.App{Name: "web", Network: "webnet"})
		assert.Contains(t, stderr(), "fly ips allocate-v6 --private --network webnet -a my-pg")
	})

	t.Run("different network with a Flycast address on the app's network", func(t *testing.T) {
		ctx, stderr := warningContext(t, &mock.FlapsClient{
			GetIPAssignmentsFunc: func(context.Context, string) (*flaps.ListIPAssignmentsResponse, error) {
				// The default network's Flycast address reports an empty name.
				return &flaps.ListIPAssignmentsResponse{IPs: []flaps.IPAssignment{flycastOn("pgnet"), flycastOn("")}}, nil
			},
		})

		warnIfUnreachable(ctx, pg, &flaps.App{Name: "web"})
		assert.Empty(t, stderr())
	})

	t.Run("lookup fails", func(t *testing.T) {
		ctx, stderr := warningContext(t, &mock.FlapsClient{
			GetIPAssignmentsFunc: func(context.Context, string) (*flaps.ListIPAssignmentsResponse, error) {
				return nil, errors.New("boom")
			},
		})

		warnIfUnreachable(ctx, pg, &flaps.App{Name: "web"})
		assert.Empty(t, stderr())
	})
}
