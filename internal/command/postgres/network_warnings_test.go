package postgres

import (
	"context"
	"errors"
	"strings"
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

func TestUnreachableWarning(t *testing.T) {
	pg := &flaps.App{Name: "my-pg", Network: "pgnet"}
	web := &flaps.App{Name: "web", Network: flapsutil.DefaultNetwork}
	pgOnly := []flaps.IPAssignment{flycastOn("pgnet")}

	t.Run("same network", func(t *testing.T) {
		assert.Empty(t, unreachableWarning(pg, &flaps.App{Name: "web", Network: "pgnet"}, nil))
		// "default" and "" both mean the default network.
		assert.Empty(t, unreachableWarning(&flaps.App{Name: "old-pg", Network: flapsutil.DefaultNetwork}, &flaps.App{Name: "web"}, nil))
	})

	t.Run("Flycast only on the cluster's network", func(t *testing.T) {
		got := unreachableWarning(pg, web, pgOnly)
		assert.Contains(t, got, "web is on network default and my-pg is on network pgnet, so web won't be able to reach the database.")
		// The default network is selected by leaving --network off.
		assert.True(t, strings.HasSuffix(got, "fly ips allocate-v6 --private -a my-pg"), got)
		assert.NotContains(t, got, "--network default")
	})

	t.Run("app on another custom network", func(t *testing.T) {
		got := unreachableWarning(pg, &flaps.App{Name: "web", Network: "webnet"}, pgOnly)
		assert.Contains(t, got, "fly ips allocate-v6 --private --network webnet -a my-pg")
	})

	t.Run("Flycast on the app's network", func(t *testing.T) {
		// The default network's Flycast address reports an empty name.
		assert.Empty(t, unreachableWarning(pg, web, []flaps.IPAssignment{flycastOn("pgnet"), flycastOn("")}))
	})

	t.Run("Flycast with no network reported", func(t *testing.T) {
		// It may be on the app's network, so don't claim it's unreachable.
		assert.Empty(t, unreachableWarning(pg, web, []flaps.IPAssignment{{IP: "fdaa:0:1::3"}}))
	})

	t.Run("no Flycast address", func(t *testing.T) {
		// Stolon and older Flex clusters connect over .internal, which never
		// crosses networks, so suggesting a Flycast address would not help.
		got := unreachableWarning(pg, web, []flaps.IPAssignment{{IP: "2a09:8280:1::1"}})
		assert.Contains(t, got, "won't be able to reach the database")
		assert.Contains(t, got, "put both apps on the same network")
		assert.NotContains(t, got, "allocate")
	})
}

func TestWarnIfUnreachableWritesToStderr(t *testing.T) {
	ctx, stderr := warningContext(t, &mock.FlapsClient{})

	warnIfUnreachable(ctx, &flaps.App{Name: "my-pg", Network: "pgnet"}, &flaps.App{Name: "web"}, []flaps.IPAssignment{flycastOn("pgnet")})
	assert.Contains(t, stderr(), "web won't be able to reach the database")
}
