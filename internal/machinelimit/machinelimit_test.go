package machinelimit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/flapsutil"
	"github.com/superfly/flyctl/internal/flyerr"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/internal/uiexutil"
	"github.com/superfly/flyctl/iostreams"
)

const request = "Request a higher limit at https://fly.io/dashboard/acme/settings?request=machine-limit"

func quota(current int, limit *int) *uiex.LaunchChecks {
	checks := &uiex.LaunchChecks{}
	checks.Checks.MachineQuota = uiex.MachineQuotaCheck{Current: current, Limit: limit}

	return checks
}

func ptr(n int) *int {
	return &n
}

func shortTimeout(t *testing.T) {
	t.Helper()

	previous := checkTimeout
	checkTimeout = 50 * time.Millisecond
	t.Cleanup(func() { checkTimeout = previous })
}

// testContext wires a ui-ex client that answers with launchChecks and counts
// how often it is asked.
func testContext(t *testing.T, jsonOutput bool, launchChecks func(context.Context) (*uiex.LaunchChecks, error)) (context.Context, func() string, *int) {
	t.Helper()

	calls := 0
	client := &mock.UiexClient{
		GetLaunchChecksFunc: func(ctx context.Context, orgSlug string) (*uiex.LaunchChecks, error) {
			calls++
			assert.Equal(t, "acme", orgSlug)

			return launchChecks(ctx)
		},
	}

	ios, _, stdout, stderr := iostreams.Test()
	t.Cleanup(func() { assert.Empty(t, stdout.String(), "machine limit notices must never go to stdout") })

	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = config.NewContext(ctx, &config.Config{JSONOutput: jsonOutput})
	ctx = uiexutil.NewContextWithClient(ctx, client)

	return ctx, stderr.String, &calls
}

func answer(checks *uiex.LaunchChecks) func(context.Context) (*uiex.LaunchChecks, error) {
	return func(context.Context) (*uiex.LaunchChecks, error) { return checks, nil }
}

func TestWarn(t *testing.T) {
	testcases := []struct {
		name     string
		checks   *uiex.LaunchChecks
		expected string
	}{
		{"below 80% stays silent", quota(79, ptr(100)), ""},
		{"80% warns", quota(80, ptr(100)), "Your organization is using 80 of its 100 Machines. " + request},
		{"between 80% and 100% warns", quota(85, ptr(100)), "Your organization is using 85 of its 100 Machines. " + request},
		{"100% says the limit is reached", quota(100, ptr(100)), "Your organization has reached its limit of 100 Machines. " + request},
		{"over the limit says the limit is reached", quota(120, ptr(100)), "Your organization has reached its limit of 100 Machines. " + request},
		{"a limit of 0 stays silent", quota(0, ptr(0)), ""},
		{"no limit stays silent", quota(500, nil), ""},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, stderr, calls := testContext(t, false, answer(tc.checks))

			Warn(ctx, "acme")

			assert.Equal(t, 1, *calls)
			if tc.expected == "" {
				assert.Empty(t, stderr())
			} else {
				assert.Contains(t, stderr(), tc.expected)
				assert.Equal(t, 1, strings.Count(stderr(), request))
			}
		})
	}
}

func TestWarnSilentWithJSONOutput(t *testing.T) {
	ctx, stderr, calls := testContext(t, true, answer(quota(100, ptr(100))))

	Warn(ctx, "acme")

	assert.Empty(t, stderr())
	assert.Zero(t, *calls, "--json should skip the request entirely")
}

func TestWarnSilentOnError(t *testing.T) {
	ctx, stderr, _ := testContext(t, false, func(context.Context) (*uiex.LaunchChecks, error) {
		return nil, errors.New("boom")
	})

	Warn(ctx, "acme")

	assert.Empty(t, stderr())
}

func TestWarnGivesUpOnSlowResponse(t *testing.T) {
	shortTimeout(t)

	ctx, stderr, _ := testContext(t, false, func(ctx context.Context) (*uiex.LaunchChecks, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			return quota(100, ptr(100)), nil
		}
	})

	start := time.Now()
	Warn(ctx, "acme")

	assert.Empty(t, stderr())
	assert.Less(t, time.Since(start), time.Second)
}

func TestWarnWithoutClientOrSlug(t *testing.T) {
	ios, _, _, stderr := iostreams.Test()
	ctx := iostreams.NewContext(context.Background(), ios)
	ctx = config.NewContext(ctx, &config.Config{})

	Warn(ctx, "acme")
	Warn(uiexutil.NewContextWithClient(ctx, &mock.UiexClient{}), "")

	assert.Empty(t, stderr.String())
}

func refusal(status int, message string) error {
	return &flaps.FlapsError{OriginalError: errors.New(message), ResponseStatusCode: status}
}

func TestRefused(t *testing.T) {
	refused := refusal(http.StatusUnprocessableEntity, "anything at all")

	assert.True(t, Refused(refused))
	assert.True(t, Refused(fmt.Errorf("could not launch machine: %w", refused)))
	assert.True(t, Refused(errors.Join(errors.New("other"), refused)))

	assert.False(t, Refused(refusal(http.StatusInternalServerError, "boom")))
	assert.False(t, Refused(errors.New("reached its machine limit")))
	assert.False(t, Refused(nil))
}

// Runs the real flaps client against flaps' actual response for an org at its
// limit, so the 422 check doesn't rest on a mock.
func TestRealFlapsLimitRefusal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":"Your organization has reached its machine limit. Please contact billing@fly.io"}`))
	}))
	t.Cleanup(server.Close)
	t.Setenv("FLY_FLAPS_BASE_URL", server.URL)

	flapsClient, err := flaps.NewWithOptions(context.Background(), flaps.NewClientOpts{Tokens: tokens.Parse("")})
	require.NoError(t, err)

	_, launchErr := flapsClient.Launch(context.Background(), "my-app", fly.LaunchMachineInput{})
	require.Error(t, launchErr)
	assert.True(t, Refused(launchErr))

	ctx, _, _ := testContext(t, false, answer(quota(20, ptr(20))))
	err = WithRequestLink(ctx, fmt.Errorf("could not launch machine: %w", launchErr), "acme")
	assert.Equal(t, request, flyerr.GetErrorSuggestion(err))
}

func TestWithRequestLink(t *testing.T) {
	// The wording is deliberately not flaps' current message: the link must
	// come from the numbers, not the text.
	refused := fmt.Errorf("could not launch machine: %w", refusal(http.StatusUnprocessableEntity, "Machine refused"))

	t.Run("refused at the limit adds the link", func(t *testing.T) {
		ctx, _, calls := testContext(t, false, answer(quota(20, ptr(20))))

		err := WithRequestLink(ctx, refused, "acme")

		require.ErrorIs(t, err, refused)
		assert.Equal(t, refused.Error(), err.Error())
		assert.Equal(t, request, flyerr.GetErrorSuggestion(err))
		assert.Equal(t, 1, *calls)
	})

	t.Run("refused over the limit adds the link", func(t *testing.T) {
		ctx, _, _ := testContext(t, false, answer(quota(25, ptr(20))))

		assert.Equal(t, request, flyerr.GetErrorSuggestion(WithRequestLink(ctx, refused, "acme")))
	})

	t.Run("refused below the limit is left alone", func(t *testing.T) {
		ctx, _, _ := testContext(t, false, answer(quota(19, ptr(20))))

		assert.Equal(t, refused, WithRequestLink(ctx, refused, "acme"))
	})

	t.Run("refused with no limit is left alone", func(t *testing.T) {
		ctx, _, _ := testContext(t, false, answer(quota(20, nil)))

		assert.Equal(t, refused, WithRequestLink(ctx, refused, "acme"))
	})

	t.Run("a failed check leaves the error alone", func(t *testing.T) {
		ctx, _, _ := testContext(t, false, func(context.Context) (*uiex.LaunchChecks, error) {
			return nil, errors.New("boom")
		})

		assert.Equal(t, refused, WithRequestLink(ctx, refused, "acme"))
	})

	t.Run("a slow check gives up and leaves the error alone", func(t *testing.T) {
		shortTimeout(t)
		ctx, _, _ := testContext(t, false, func(ctx context.Context) (*uiex.LaunchChecks, error) {
			<-ctx.Done()

			return nil, ctx.Err()
		})

		start := time.Now()
		assert.Equal(t, refused, WithRequestLink(ctx, refused, "acme"))
		assert.Less(t, time.Since(start), time.Second)
	})

	t.Run("other errors skip the check", func(t *testing.T) {
		ctx, _, calls := testContext(t, false, answer(quota(20, ptr(20))))

		serverErr := refusal(http.StatusInternalServerError, "boom")
		assert.Equal(t, serverErr, WithRequestLink(ctx, serverErr, "acme"))
		assert.NoError(t, WithRequestLink(ctx, nil, "acme"))
		assert.Zero(t, *calls)
	})

	t.Run("no org leaves the error alone", func(t *testing.T) {
		ctx, _, calls := testContext(t, false, answer(quota(20, ptr(20))))

		assert.Equal(t, refused, WithRequestLink(ctx, refused, ""))
		assert.Zero(t, *calls)
	})
}

func TestAppOrgSlug(t *testing.T) {
	withFlaps := func(getApp func(context.Context, string) (*flaps.App, error)) context.Context {
		return flapsutil.NewContextWithClient(context.Background(), &mock.FlapsClient{GetAppFunc: getApp})
	}

	ctx := withFlaps(func(_ context.Context, name string) (*flaps.App, error) {
		assert.Equal(t, "my-app", name)

		return &flaps.App{Name: name, Organization: flaps.AppOrganizationInfo{Slug: "acme"}}, nil
	})
	assert.Equal(t, "acme", AppOrgSlug(ctx, "my-app"))

	ctx = withFlaps(func(context.Context, string) (*flaps.App, error) { return nil, errors.New("boom") })
	assert.Empty(t, AppOrgSlug(ctx, "my-app"))

	shortTimeout(t)
	ctx = withFlaps(func(ctx context.Context, _ string) (*flaps.App, error) {
		<-ctx.Done()

		return nil, ctx.Err()
	})
	start := time.Now()
	assert.Empty(t, AppOrgSlug(ctx, "my-app"))
	assert.Less(t, time.Since(start), time.Second)

	assert.Empty(t, AppOrgSlug(context.Background(), "my-app"))
}
