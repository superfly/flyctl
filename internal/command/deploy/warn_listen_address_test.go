package deploy

import (
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/machine"
	"github.com/superfly/flyctl/iostreams"
)

// newListenAddressDeployment builds a machineDeployment sized down for testing
// warnAboutIncorrectListenAddress. The app is configured with a single TCP
// service on port 3000 (the same shape as the fly.toml in
// https://github.com/superfly/flyctl/issues/3358).
func newListenAddressDeployment(t *testing.T, client *mockFlapsClient) (*machineDeployment, *iostreams.IOStreams, func() string) {
	t.Helper()

	ios, _, _, stderr := iostreams.Test()

	cfg := appconfig.NewConfig()
	cfg.AppName = "test-app"
	cfg.HTTPService = &appconfig.HTTPService{InternalPort: 3000}
	require.NoError(t, cfg.SetMachinesPlatform())

	md := &machineDeployment{
		app:         &flaps.App{Name: "test-app"},
		io:          ios,
		colorize:    ios.ColorScheme(),
		flapsClient: client,
		appConfig:   cfg,
	}

	return md, ios, func() string { return stderr.String() }
}

func processesWithListener(port int) fly.MachinePsResponse {
	return fly.MachinePsResponse{
		{
			Pid:     1,
			Command: "puma",
			ListenSockets: []fly.ListenSocket{
				{Proto: "tcp", Address: "0.0.0.0:" + strconv.Itoa(port)},
			},
		},
	}
}

func processesWithoutListener() fly.MachinePsResponse {
	return fly.MachinePsResponse{
		{
			Pid:     1,
			Command: "puma",
			// The init still reports SSH etc. so we get a non-zero foundSockets
			// value, otherwise the old-init fallback path would kick in and hide
			// the warning entirely.
			ListenSockets: []fly.ListenSocket{
				{Proto: "tcp", Address: "127.0.0.1:22"},
			},
		},
	}
}

// TestWarnAboutIncorrectListenAddress_WaitsForSlowBind is the regression test
// for https://github.com/superfly/flyctl/issues/3358. Apps like Rails/Puma or
// NodeJS+puppeteer can take several seconds to bind after the machine is
// considered started. flyctl used to warn "The app is not listening on the
// expected address" the first time it looked, before the app had a chance to
// bind. This test simulates a slow bind and asserts that no warning is emitted.
func TestWarnAboutIncorrectListenAddress_WaitsForSlowBind(t *testing.T) {
	// Give the polling loop enough time to observe the delayed bind.
	prev := defaultListenAddressCheckTimeout
	defaultListenAddressCheckTimeout = 5 * time.Second
	t.Cleanup(func() { defaultListenAddressCheckTimeout = prev })

	var calls atomic.Int32
	// Simulate an app that hasn't bound to its port yet for the first few
	// polls, then binds to 0.0.0.0:3000.
	client := &mockFlapsClient{
		GetProcessesFunc: func(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
			n := calls.Add(1)
			if n < 3 {
				return processesWithoutListener(), nil
			}

			return processesWithListener(3000), nil
		},
	}

	md, ios, stderr := newListenAddressDeployment(t, client)
	lm := machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: "m1"}, false)

	md.warnAboutIncorrectListenAddress(context.Background(), lm)

	assert.NotContains(t, stderr(), "not listening on the expected address",
		"warning should be suppressed once the app binds; got: %s", stderr())
	assert.GreaterOrEqual(t, calls.Load(), int32(3),
		"polling should retry until the expected address appears")
}

// TestWarnAboutIncorrectListenAddress_SucceedsImmediately covers the fast
// path where the app is already listening on the expected address by the time
// we check. We should not spend any time polling in this case.
func TestWarnAboutIncorrectListenAddress_SucceedsImmediately(t *testing.T) {
	prev := defaultListenAddressCheckTimeout
	defaultListenAddressCheckTimeout = 5 * time.Second
	t.Cleanup(func() { defaultListenAddressCheckTimeout = prev })

	var calls atomic.Int32
	client := &mockFlapsClient{
		GetProcessesFunc: func(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
			calls.Add(1)

			return processesWithListener(3000), nil
		},
	}

	md, ios, stderr := newListenAddressDeployment(t, client)
	lm := machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: "m1"}, false)

	start := time.Now()
	md.warnAboutIncorrectListenAddress(context.Background(), lm)
	elapsed := time.Since(start)

	assert.NotContains(t, stderr(), "not listening on the expected address")
	assert.Equal(t, int32(1), calls.Load(),
		"should only poll once when the address is already listening")
	assert.Less(t, elapsed, 500*time.Millisecond,
		"the happy path should return quickly, took %s", elapsed)
}

// TestWarnAboutIncorrectListenAddress_WarnsAfterTimeout ensures that if the
// app truly never binds to the expected address, we still emit the warning
// with the port that is missing.
func TestWarnAboutIncorrectListenAddress_WarnsAfterTimeout(t *testing.T) {
	prev := defaultListenAddressCheckTimeout
	// Keep the timeout small so the test runs quickly but still exercises the
	// polling loop.
	defaultListenAddressCheckTimeout = 300 * time.Millisecond
	t.Cleanup(func() { defaultListenAddressCheckTimeout = prev })

	client := &mockFlapsClient{
		GetProcessesFunc: func(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
			return processesWithoutListener(), nil
		},
	}

	md, ios, stderr := newListenAddressDeployment(t, client)
	lm := machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: "m1"}, false)

	md.warnAboutIncorrectListenAddress(context.Background(), lm)

	assert.Contains(t, stderr(), "not listening on the expected address")
	assert.Contains(t, stderr(), "0.0.0.0:3000",
		"the missing port should be reported in the warning")
}

// TestWarnAboutIncorrectListenAddress_OnlyChecksOncePerGroup verifies that
// the sync.Map-based caching still works: a second call for the same process
// group should be a no-op even if we would otherwise poll.
func TestWarnAboutIncorrectListenAddress_OnlyChecksOncePerGroup(t *testing.T) {
	prev := defaultListenAddressCheckTimeout
	defaultListenAddressCheckTimeout = 5 * time.Second
	t.Cleanup(func() { defaultListenAddressCheckTimeout = prev })

	var calls atomic.Int32
	client := &mockFlapsClient{
		GetProcessesFunc: func(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
			calls.Add(1)

			return processesWithListener(3000), nil
		},
	}

	md, ios, _ := newListenAddressDeployment(t, client)
	lm := machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: "m1"}, false)

	md.warnAboutIncorrectListenAddress(context.Background(), lm)
	md.warnAboutIncorrectListenAddress(context.Background(), lm)

	assert.Equal(t, int32(1), calls.Load(),
		"the second call for the same process group should short-circuit")
}

// TestListenAddressCheckTimeoutFor_UsesLongestGracePeriod verifies that when
// the user has configured a check with a `grace_period` larger than the
// default polling window, we honour it. This matters for apps that legitimately
// take a long time to boot: the user has already told us how long, so we
// should trust that value rather than fire a spurious "not listening" warning.
func TestListenAddressCheckTimeoutFor_UsesLongestGracePeriod(t *testing.T) {
	prev := defaultListenAddressCheckTimeout
	defaultListenAddressCheckTimeout = 5 * time.Second
	t.Cleanup(func() { defaultListenAddressCheckTimeout = prev })

	t.Run("nil config falls back to default", func(t *testing.T) {
		assert.Equal(t, defaultListenAddressCheckTimeout, listenAddressCheckTimeoutFor(nil))
	})

	t.Run("no checks configured falls back to default", func(t *testing.T) {
		cfg := appconfig.NewConfig()
		cfg.HTTPService = &appconfig.HTTPService{InternalPort: 3000}
		require.NoError(t, cfg.SetMachinesPlatform())

		assert.Equal(t, defaultListenAddressCheckTimeout, listenAddressCheckTimeoutFor(cfg))
	})

	t.Run("shorter grace period does not lower the default", func(t *testing.T) {
		cfg := appconfig.NewConfig()
		cfg.HTTPService = &appconfig.HTTPService{
			InternalPort: 3000,
			HTTPChecks: []*appconfig.ServiceHTTPCheck{
				{GracePeriod: fly.MustParseDuration("2s")},
			},
		}
		require.NoError(t, cfg.SetMachinesPlatform())

		assert.Equal(t, defaultListenAddressCheckTimeout, listenAddressCheckTimeoutFor(cfg))
	})

	t.Run("http_service check grace period is honoured", func(t *testing.T) {
		cfg := appconfig.NewConfig()
		cfg.HTTPService = &appconfig.HTTPService{
			InternalPort: 3000,
			HTTPChecks: []*appconfig.ServiceHTTPCheck{
				{GracePeriod: fly.MustParseDuration("45s")},
			},
		}
		require.NoError(t, cfg.SetMachinesPlatform())

		assert.Equal(t, 45*time.Second, listenAddressCheckTimeoutFor(cfg))
	})

	t.Run("top-level [checks] grace period is honoured", func(t *testing.T) {
		cfg := appconfig.NewConfig()
		cfg.HTTPService = &appconfig.HTTPService{InternalPort: 3000}
		cfg.Checks = map[string]*appconfig.ToplevelCheck{
			"alive": {GracePeriod: fly.MustParseDuration("90s")},
		}
		require.NoError(t, cfg.SetMachinesPlatform())

		assert.Equal(t, 90*time.Second, listenAddressCheckTimeoutFor(cfg))
	})

	t.Run("service tcp_checks grace period is honoured", func(t *testing.T) {
		cfg := appconfig.NewConfig()
		cfg.Services = []appconfig.Service{{
			Protocol:     "tcp",
			InternalPort: 5432,
			TCPChecks: []*appconfig.ServiceTCPCheck{
				{GracePeriod: fly.MustParseDuration("30s")},
			},
		}}
		require.NoError(t, cfg.SetMachinesPlatform())

		assert.Equal(t, 30*time.Second, listenAddressCheckTimeoutFor(cfg))
	})

	t.Run("largest grace period across checks wins", func(t *testing.T) {
		cfg := appconfig.NewConfig()
		cfg.HTTPService = &appconfig.HTTPService{
			InternalPort: 3000,
			HTTPChecks: []*appconfig.ServiceHTTPCheck{
				{GracePeriod: fly.MustParseDuration("20s")},
				{GracePeriod: fly.MustParseDuration("60s")},
			},
		}
		cfg.Checks = map[string]*appconfig.ToplevelCheck{
			"alive": {GracePeriod: fly.MustParseDuration("40s")},
		}
		require.NoError(t, cfg.SetMachinesPlatform())

		assert.Equal(t, 60*time.Second, listenAddressCheckTimeoutFor(cfg))
	})
}

// TestWarnAboutIncorrectListenAddress_HonorsGracePeriod is an end-to-end check
// that ties the grace-period plumbing to the polling loop. It configures a
// grace_period of 2s, then feeds a mock that only reports a bound listener
// after ~1s. With the default 5ms timeout in this test, the poll would give
// up long before the bind; with grace_period honoured, it should wait and
// suppress the warning.
func TestWarnAboutIncorrectListenAddress_HonorsGracePeriod(t *testing.T) {
	prev := defaultListenAddressCheckTimeout
	// Set the default extremely low so the test only passes if grace_period is
	// actually being consulted.
	defaultListenAddressCheckTimeout = 5 * time.Millisecond
	t.Cleanup(func() { defaultListenAddressCheckTimeout = prev })

	start := time.Now()
	client := &mockFlapsClient{
		GetProcessesFunc: func(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
			if time.Since(start) < 1*time.Second {
				return processesWithoutListener(), nil
			}

			return processesWithListener(3000), nil
		},
	}

	ios, _, _, stderr := iostreams.Test()
	cfg := appconfig.NewConfig()
	cfg.AppName = "test-app"
	cfg.HTTPService = &appconfig.HTTPService{
		InternalPort: 3000,
		HTTPChecks: []*appconfig.ServiceHTTPCheck{
			{GracePeriod: fly.MustParseDuration("2s")},
		},
	}
	require.NoError(t, cfg.SetMachinesPlatform())

	md := &machineDeployment{
		app:         &flaps.App{Name: "test-app"},
		io:          ios,
		colorize:    ios.ColorScheme(),
		flapsClient: client,
		appConfig:   cfg,
	}
	lm := machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: "m1"}, false)

	md.warnAboutIncorrectListenAddress(context.Background(), lm)

	assert.NotContains(t, stderr.String(), "not listening on the expected address",
		"grace_period should extend the polling window so slow-binding apps aren't flagged; got: %s", stderr.String())
}

// TestWarnAboutIncorrectListenAddress_HonorsContextCancellation asserts that
// the polling loop bails out promptly if the caller's context is cancelled
// (e.g. the user hit Ctrl-C during deploy).
func TestWarnAboutIncorrectListenAddress_HonorsContextCancellation(t *testing.T) {
	prev := defaultListenAddressCheckTimeout
	defaultListenAddressCheckTimeout = 30 * time.Second
	t.Cleanup(func() { defaultListenAddressCheckTimeout = prev })

	var (
		mu    sync.Mutex
		calls int
	)
	client := &mockFlapsClient{
		GetProcessesFunc: func(ctx context.Context, appName, machineID string) (fly.MachinePsResponse, error) {
			mu.Lock()
			calls++
			mu.Unlock()

			return processesWithoutListener(), nil
		},
	}

	md, ios, _ := newListenAddressDeployment(t, client)
	lm := machine.NewLeasableMachine(client, ios, "test-app", &fly.Machine{ID: "m1"}, false)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel shortly after we start to interrupt the polling loop.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	md.warnAboutIncorrectListenAddress(ctx, lm)
	elapsed := time.Since(start)

	assert.Less(t, elapsed, 5*time.Second,
		"cancelled context should cut the polling short, took %s", elapsed)
}
