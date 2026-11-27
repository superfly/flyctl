package config

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
)

func noJitter(d time.Duration) time.Duration { return d }

func TestOrgCheckSchedule(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	users := []string{"fo1_user"}
	s := newOrgCheckSchedule(start, users, noJitter)

	require.False(t, s.due(start.Add(orgCheckInterval-time.Second), users))
	require.True(t, s.due(start.Add(orgCheckInterval), users))

	// A different login is checked right away.
	require.True(t, s.due(start.Add(time.Second), []string{"fo1_other"}))

	// A failed check is retried sooner, and still counts as a check of those
	// users, so it isn't repeated on every tick.
	s.done(start, []string{"fo1_other"}, errors.New("web is down"))
	require.False(t, s.due(start.Add(orgCheckRetryInterval-time.Second), []string{"fo1_other"}))
	require.True(t, s.due(start.Add(orgCheckRetryInterval), []string{"fo1_other"}))
}

func TestJitterTenPercent(t *testing.T) {
	for range 1000 {
		d := jitterTenPercent(orgCheckInterval)
		require.GreaterOrEqual(t, d, orgCheckInterval-orgCheckInterval/10)
		require.LessOrEqual(t, d, orgCheckInterval+orgCheckInterval/10)
	}
}

// runTokenRefresh runs the background loop in the current synctest bubble for
// d, then stops it. It returns when the loop has exited. Each call to the org
// check gets the next error in orgErrs, or nil once they run out. during runs
// while the loop does.
func runTokenRefresh(t *testing.T, toks *tokens.Tokens, d time.Duration, orgErrs []error, during func()) (orgChecks, discharges []time.Duration) {
	t.Helper()

	ctx, cancel := context.WithCancel(logger.NewContext(t.Context(), logger.New(io.Discard, logger.Debug, false)))
	defer cancel()

	start := time.Now()
	ops := tokenRefreshOps{
		fetchOrgTokens: func(_ context.Context, _ *tokens.Tokens, trigger orgCheckTrigger) (bool, error) {
			assert.Equal(t, orgCheckPeriodic, trigger)
			orgChecks = append(orgChecks, time.Since(start))
			if len(orgErrs) == 0 {
				return false, nil
			}
			err := orgErrs[0]
			orgErrs = orgErrs[1:]

			return false, err
		},
		refreshDischargeTokens: func(context.Context, *tokens.Tokens, UserURLCallback, time.Duration) (bool, error) {
			discharges = append(discharges, time.Since(start))

			return false, nil
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- refreshTokensPeriodically(ctx, &tokenMonitor{disk: toks.Copy()}, toks, nil, ops, noJitter)
	}()

	if during != nil {
		go during()
	}
	time.Sleep(d)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)

	return orgChecks, discharges
}

func TestRefreshTokensPeriodically(t *testing.T) {
	const minute = time.Minute

	t.Run("checks organizations every 15 minutes and discharges every minute", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			orgChecks, discharges := runTokenRefresh(t, tokens.Parse("fm2_x,fo1_user"), 59*minute+30*time.Second, nil, nil)

			require.Equal(t, []time.Duration{15 * minute, 30 * minute, 45 * minute}, orgChecks)
			require.Len(t, discharges, 59)
			require.Equal(t, minute, discharges[0])
		})
	})

	t.Run("retries a failed organization check after 5 minutes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			orgErrs := []error{errors.New("web is down")}
			orgChecks, discharges := runTokenRefresh(t, tokens.Parse("fm2_x,fo1_user"), 36*minute+30*time.Second, orgErrs, nil)

			require.Equal(t, []time.Duration{15 * minute, 20 * minute, 35 * minute}, orgChecks)
			require.Len(t, discharges, 36)
		})
	})

	t.Run("checks organizations on the next tick after a different login", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			toks := tokens.Parse("fm2_x,fo1_user")
			relogin := func() {
				time.Sleep(3*minute + 30*time.Second)
				toks.Replace(tokens.Parse("fm2_y,fo1_other"))
			}
			orgChecks, _ := runTokenRefresh(t, toks, 20*minute+30*time.Second, nil, relogin)

			require.Equal(t, []time.Duration{4 * minute, 19 * minute}, orgChecks)
		})
	})
}

func TestDefaultOrgFetcher(t *testing.T) {
	ctx := logger.NewContext(t.Context(), logger.New(io.Discard, logger.Debug, false))

	type request struct {
		Query         string `json:"query"`
		OperationName string `json:"operationName"`
	}
	serve := func(t *testing.T, status int, body string) (flyutil.Client, *request) {
		t.Helper()

		var got request
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = io.WriteString(w, body)
		}))
		t.Cleanup(srv.Close)

		return flyutil.NewClientFromOptions(ctx, fly.ClientOptions{BaseURL: srv.URL, Tokens: tokens.Parse("fo1_x")}), &got
	}

	for _, tc := range []struct {
		trigger   orgCheckTrigger
		operation string
	}{
		{orgCheckStartup, "OrgTokenCheckStartup"},
		{orgCheckPeriodic, "OrgTokenCheckPeriodic"},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			// web sends internalNumericId, a BigInt, as a JSON string.
			c, got := serve(t, http.StatusOK, `{"data":{"organizations":{"nodes":[
				{"id":"graph-1","internalNumericId":"1"},
				{"id":"graph-2","internalNumericId":"9007199254740993"}
			]}}}`)

			orgs, err := defaultOrgFetcher(ctx, c, tc.trigger)

			require.NoError(t, err)
			require.Equal(t, map[uint64]string{1: "graph-1", 9007199254740993: "graph-2"}, orgs)
			require.Equal(t, tc.operation, got.OperationName)
			require.Contains(t, got.Query, "query "+tc.operation)
			require.Contains(t, got.Query, "internalNumericId")
			for _, field := range []string{"viewerRole", "billable", "paidPlan", "slug", "name"} {
				require.NotContains(t, got.Query, field)
			}
		})
	}

	t.Run("GraphQL error", func(t *testing.T) {
		c, _ := serve(t, http.StatusOK, `{"data":null,"errors":[{"message":"You must be authenticated to view this."}]}`)

		orgs, err := defaultOrgFetcher(ctx, c, orgCheckPeriodic)

		require.Error(t, err)
		require.Nil(t, orgs)
	})

	t.Run("HTTP error", func(t *testing.T) {
		c, _ := serve(t, http.StatusUnauthorized, `{"message":"Unauthorized"}`)

		orgs, err := defaultOrgFetcher(ctx, c, orgCheckStartup)

		require.Error(t, err)
		require.Nil(t, orgs)
	})
}
