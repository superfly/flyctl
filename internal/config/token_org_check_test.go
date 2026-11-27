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
	const minute = time.Minute

	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	users := []string{"fo1_user"}
	failed := errors.New("failed")
	checkedAtStart := func() *orgCheckSchedule {
		s := newOrgCheckSchedule(noJitter)
		s.done(start, users, nil)

		return s
	}

	t.Run("a new schedule is due", func(t *testing.T) {
		require.True(t, newOrgCheckSchedule(noJitter).due(start, users))
	})

	t.Run("due again after the interval, on the nearest tick", func(t *testing.T) {
		s := checkedAtStart()

		require.False(t, s.due(start.Add(orgCheckInterval-minute), users))
		require.True(t, s.due(start.Add(orgCheckInterval-orgCheckSlack), users))
		require.True(t, s.due(start.Add(orgCheckInterval), users))
	})

	t.Run("a different login is checked right away", func(t *testing.T) {
		require.True(t, checkedAtStart().due(start.Add(time.Second), []string{"fo1_other"}))
	})

	t.Run("a failed check is retried sooner, not on every tick", func(t *testing.T) {
		s := newOrgCheckSchedule(noJitter)
		s.done(start, users, failed)

		require.False(t, s.due(start.Add(orgCheckRetryInterval-minute), users))
		require.True(t, s.due(start.Add(orgCheckRetryInterval), users))
	})

	// time.Now carries a monotonic reading, and that clock stops while a
	// laptop sleeps. Comparing with it would leave the check waiting out the
	// rest of its 15 awake minutes after a night with the lid closed.
	t.Run("keeps wall-clock time, so time asleep counts", func(t *testing.T) {
		now := time.Now()
		require.Contains(t, now.String(), " m=")

		s := newOrgCheckSchedule(noJitter)
		s.done(now, users, nil)

		require.NotContains(t, s.checked.String(), " m=")
		require.NotContains(t, s.next.String(), " m=")
		require.False(t, s.due(now.Add(minute), users))
		require.True(t, s.due(now.Add(orgCheckInterval), users))
	})

	t.Run("a clock set back to before the last check is checked right away", func(t *testing.T) {
		require.True(t, checkedAtStart().due(start.Add(-time.Hour), users))
	})

	t.Run("a dropped result is due again", func(t *testing.T) {
		s := checkedAtStart()
		s.resultDropped()

		require.True(t, s.due(start.Add(minute), users))
	})

	t.Run("a discharge that starts failing brings the check forward, once", func(t *testing.T) {
		s := checkedAtStart()
		s.dischargeResult(nil)
		require.False(t, s.due(start.Add(orgCheckRetryInterval), users))

		// No sooner than the retry interval after the last check.
		s.dischargeResult(failed)
		require.False(t, s.due(start.Add(orgCheckRetryInterval-minute), users))
		require.True(t, s.due(start.Add(orgCheckRetryInterval), users))

		// The check ran and the discharge still fails, so it isn't about an
		// organization: back to the usual interval.
		checked := start.Add(orgCheckRetryInterval)
		s.done(checked, users, nil)
		s.dischargeResult(failed)
		require.False(t, s.due(checked.Add(orgCheckInterval-minute), users))
		require.True(t, s.due(checked.Add(orgCheckInterval), users))

		// A new run of failures counts again.
		s.dischargeResult(nil)
		s.dischargeResult(failed)
		require.True(t, s.due(checked.Add(orgCheckRetryInterval), users))
	})
}

func TestJitterTenPercent(t *testing.T) {
	for range 1000 {
		d := jitterTenPercent(orgCheckInterval)
		require.GreaterOrEqual(t, d, orgCheckInterval-orgCheckInterval/10)
		require.LessOrEqual(t, d, orgCheckInterval+orgCheckInterval/10)
	}
}

// orgCheckLatency is how long the fake organization check takes. A real one is
// a round trip to web, and a check that takes no time hides scheduling bugs.
const orgCheckLatency = 300 * time.Millisecond

// tokenRefreshRun is one run of the monitor in a synctest bubble: the start-up
// pass, then the background loop.
type tokenRefreshRun struct {
	toks *tokens.Tokens
	// startupErr is what the start-up organization check returns.
	startupErr error
	// orgCheck stands in for the nth periodic organization check, from 0. It
	// gets the copy of the tokens that the loop is refreshing.
	orgCheck func(n int, refreshing *tokens.Tokens) (bool, error)
	// dischargeErr is what a discharge refresh returns at the given time since
	// the loop started.
	dischargeErr func(since time.Duration) error
	// during runs alongside the loop.
	during func()
}

// run runs the monitor for d, then stops it. It returns when the periodic
// organization checks and the discharge refreshes started, since the start.
func (r tokenRefreshRun) run(t *testing.T, d time.Duration) (orgChecks, discharges []time.Duration) {
	t.Helper()

	ctx, cancel := context.WithCancel(logger.NewContext(t.Context(), logger.New(io.Discard, logger.Debug, false)))
	defer cancel()

	start := time.Now()
	m := &tokenMonitor{disk: r.toks.Copy()}

	startup := tokenRefreshOps{
		fetchOrgTokens: func(_ context.Context, _ *tokens.Tokens, trigger orgCheckTrigger) (bool, error) {
			assert.Equal(t, orgCheckStartup, trigger)

			return false, r.startupErr
		},
		refreshDischargeTokens: func(context.Context, *tokens.Tokens, UserURLCallback, time.Duration) (bool, error) {
			return false, nil
		},
	}
	orgCheck, err := refreshTokensAtStartup(ctx, m, r.toks, nil, startup, noJitter)
	require.NoError(t, err)

	periodic := tokenRefreshOps{
		fetchOrgTokens: func(_ context.Context, refreshing *tokens.Tokens, trigger orgCheckTrigger) (bool, error) {
			assert.Equal(t, orgCheckPeriodic, trigger)
			n := len(orgChecks)
			orgChecks = append(orgChecks, time.Since(start))
			time.Sleep(orgCheckLatency)
			if r.orgCheck == nil {
				return false, nil
			}

			return r.orgCheck(n, refreshing)
		},
		refreshDischargeTokens: func(context.Context, *tokens.Tokens, UserURLCallback, time.Duration) (bool, error) {
			since := time.Since(start)
			discharges = append(discharges, since)
			if r.dischargeErr == nil {
				return false, nil
			}

			return false, r.dischargeErr(since)
		},
	}

	done := make(chan error, 1)
	go func() {
		done <- refreshTokensPeriodically(ctx, m, r.toks, nil, periodic, orgCheck)
	}()

	if r.during != nil {
		go r.during()
	}
	time.Sleep(d)
	cancel()
	require.ErrorIs(t, <-done, context.Canceled)

	return orgChecks, discharges
}

func TestRefreshTokensPeriodically(t *testing.T) {
	const (
		minute = time.Minute
		// halfTick lets a run end between two ticks.
		halfTick = 30 * time.Second
	)

	webIsDown := errors.New("web is down")

	t.Run("checks organizations every 15 minutes and discharges every minute", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			run := tokenRefreshRun{toks: tokens.Parse("fm2_x,fo1_user")}
			orgChecks, discharges := run.run(t, 59*minute+halfTick)

			require.Equal(t, []time.Duration{15 * minute, 30 * minute, 45 * minute}, orgChecks)
			require.Len(t, discharges, 59)
			require.Equal(t, minute, discharges[0])
		})
	})

	t.Run("retries a failed organization check after 5 minutes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			run := tokenRefreshRun{
				toks: tokens.Parse("fm2_x,fo1_user"),
				orgCheck: func(n int, _ *tokens.Tokens) (bool, error) {
					if n == 0 {
						return false, webIsDown
					}

					return false, nil
				},
			}
			orgChecks, discharges := run.run(t, 36*minute+halfTick)

			require.Equal(t, []time.Duration{15 * minute, 20 * minute, 35 * minute}, orgChecks)
			require.Len(t, discharges, 36)
		})
	})

	t.Run("retries a failed start-up check after 5 minutes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			run := tokenRefreshRun{toks: tokens.Parse("fm2_x,fo1_user"), startupErr: webIsDown}
			orgChecks, _ := run.run(t, 21*minute+halfTick)

			require.Equal(t, []time.Duration{5 * minute, 20 * minute}, orgChecks)
		})
	})

	t.Run("checks organizations on the next tick after a different login", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			toks := tokens.Parse("fm2_x,fo1_user")
			run := tokenRefreshRun{
				toks: toks,
				during: func() {
					time.Sleep(3*minute + halfTick)
					toks.Replace(tokens.Parse("fm2_y,fo1_other"))
				},
			}
			orgChecks, _ := run.run(t, 20*minute+halfTick)

			require.Equal(t, []time.Duration{4 * minute, 19 * minute}, orgChecks)
		})
	})

	// Another flyctl process saved the config file while the check ran and the
	// watcher adopted it, so the commit keeps those tokens and drops what the
	// check minted. Same login, so nothing else would bring the check back.
	t.Run("checks again on the next tick when the result was dropped", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			toks := fakeTokens(t, "fo1_user", 1)
			fromOtherProcess := fakeTokens(t, "fo1_user", 1)
			run := tokenRefreshRun{
				toks: toks,
				orgCheck: func(n int, refreshing *tokens.Tokens) (bool, error) {
					if n > 0 {
						return false, nil
					}
					toks.Replace(fromOtherProcess)
					refreshing.Replace(fakeTokens(t, "fo1_user", 1, 2))

					return true, nil
				},
			}
			orgChecks, _ := run.run(t, 32*minute+halfTick)

			require.Equal(t, []time.Duration{15 * minute, 16 * minute, 31 * minute}, orgChecks)
			assertTokenOrgs(t, toks, 1)
		})
	})

	// The token of an organization the user left fails to discharge on every
	// tick until the check prunes it.
	for _, tc := range []struct {
		name         string
		failingSince time.Duration
		orgChecks    []time.Duration
	}{
		// Brought forward once. The discharges keep failing after that check,
		// and the next one is a full interval later.
		{"on the next tick", 7 * time.Minute, []time.Duration{8 * minute, 23 * minute, 38 * minute}},
		{"no sooner than 5 minutes after the last check", 2 * time.Minute, []time.Duration{5 * minute, 20 * minute, 35 * minute}},
	} {
		t.Run("checks organizations when discharges start failing: "+tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				run := tokenRefreshRun{
					toks: tokens.Parse("fm2_x,fo1_user"),
					dischargeErr: func(since time.Duration) error {
						if since < tc.failingSince {
							return nil
						}

						return errors.New("discharge refused")
					},
				}
				orgChecks, _ := run.run(t, 39*minute+halfTick)

				require.Equal(t, tc.orgChecks, orgChecks)
			})
		})
	}
}

func TestRefreshTokensAtStartup(t *testing.T) {
	ctx := logger.NewContext(t.Context(), logger.New(io.Discard, logger.Debug, false))

	// A login made while the start-up check ran: the commit keeps the login
	// and drops what the check minted for the old one.
	toks := fakeTokens(t, "fo1_user", 1)
	login := fakeTokens(t, "fo1_other", 3)
	ops := tokenRefreshOps{
		fetchOrgTokens: func(_ context.Context, refreshing *tokens.Tokens, _ orgCheckTrigger) (bool, error) {
			toks.Replace(login)
			refreshing.Replace(fakeTokens(t, "fo1_user", 1, 2))

			return true, nil
		},
		refreshDischargeTokens: func(context.Context, *tokens.Tokens, UserURLCallback, time.Duration) (bool, error) {
			return false, errors.New("needs a browser")
		},
	}

	orgCheck, err := refreshTokensAtStartup(ctx, &tokenMonitor{disk: toks.Copy()}, toks, nil, ops, noJitter)

	require.Error(t, err)
	require.True(t, toks.Equal(login))
	require.True(t, orgCheck.due(time.Now(), []string{"fo1_user"}), "a dropped result is checked again")
	require.True(t, orgCheck.dischargeFailing)
}

func TestEnsureOrgToken(t *testing.T) {
	ctx := logger.NewContext(t.Context(), logger.New(io.Discard, logger.Debug, false))

	monitored := func(t *testing.T, toks *tokens.Tokens) *tokens.Tokens {
		t.Helper()
		tokenMonitors.Store(toks, &tokenMonitor{disk: toks.Copy()})
		t.Cleanup(func() { tokenMonitors.Delete(toks) })

		return toks
	}
	// mintsOrg2 is a check that finds the user in organizations 1 and 2.
	mintsOrg2 := func(t *testing.T, checks *int) tokenRefreshOps {
		t.Helper()

		return tokenRefreshOps{
			fetchOrgTokens: func(ctx context.Context, refreshing *tokens.Tokens, trigger orgCheckTrigger) (bool, error) {
				*checks++
				assert.Equal(t, orgCheckStartup, trigger)
				fetchOrgs := fakeOrgFetcher(map[uint64]string{1: "org1", 2: "org2"}, nil)

				return doFetchOrgTokens(ctx, refreshing, fetchOrgs, fakeOrgTokenMinter(t, "org2", 2))
			},
			refreshDischargeTokens: func(_ context.Context, _ *tokens.Tokens, uucb UserURLCallback, _ time.Duration) (bool, error) {
				assert.Nil(t, uucb, "must not send the user to a browser")

				return false, nil
			},
		}
	}

	t.Run("fetches the token of an organization it doesn't hold", func(t *testing.T) {
		var checks int
		toks := monitored(t, fakeTokens(t, "fo1_user", 1))

		require.NoError(t, ensureOrgToken(ctx, toks, 2, mintsOrg2(t, &checks)))

		require.Equal(t, 1, checks)
		assertTokenOrgs(t, toks, 1, 2)

		// Now that it holds the token, asking again costs nothing.
		require.NoError(t, ensureOrgToken(ctx, toks, 2, mintsOrg2(t, &checks)))
		require.Equal(t, 1, checks)
	})

	t.Run("asks nothing when there is nothing to fetch", func(t *testing.T) {
		var checks int
		toks := monitored(t, fakeTokens(t, "fo1_user", 1))

		require.NoError(t, ensureOrgToken(ctx, toks, 1, mintsOrg2(t, &checks)), "already held")
		require.NoError(t, ensureOrgToken(ctx, toks, 0, mintsOrg2(t, &checks)), "unknown organization")
		require.NoError(t, ensureOrgToken(ctx, fakeTokens(t, "fo1_user", 1), 2, mintsOrg2(t, &checks)), "tokens nobody monitors")

		require.Zero(t, checks)
	})

	t.Run("reports a failed check and keeps the tokens", func(t *testing.T) {
		toks := monitored(t, fakeTokens(t, "fo1_user", 1))
		webIsDown := errors.New("web is down")
		ops := tokenRefreshOps{
			fetchOrgTokens: func(context.Context, *tokens.Tokens, orgCheckTrigger) (bool, error) {
				return false, webIsDown
			},
		}

		require.ErrorIs(t, ensureOrgToken(ctx, toks, 2, ops), webIsDown)
		assertTokenOrgs(t, toks, 1)
	})

	t.Run("reports a result that a new login made void", func(t *testing.T) {
		var checks int
		toks := monitored(t, fakeTokens(t, "fo1_user", 1))
		login := fakeTokens(t, "fo1_other", 3)
		ops := mintsOrg2(t, &checks)
		check := ops.fetchOrgTokens
		ops.fetchOrgTokens = func(ctx context.Context, refreshing *tokens.Tokens, trigger orgCheckTrigger) (bool, error) {
			toks.Replace(login)

			return check(ctx, refreshing, trigger)
		}

		require.Error(t, ensureOrgToken(ctx, toks, 2, ops))
		require.True(t, toks.Equal(login))
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

	triggers := []struct {
		trigger   orgCheckTrigger
		operation string
	}{
		{orgCheckStartup, "OrgTokenCheckStartup"},
		{orgCheckPeriodic, "OrgTokenCheckPeriodic"},
	}

	for _, tc := range triggers {
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

	// The caller prunes the token of every organization the answer leaves out,
	// so an answer that isn't a complete list must not look like an empty one.
	for name, body := range map[string]string{
		"no data":                `{"data":null}`,
		"empty object":           `{}`,
		"no organizations field": `{"data":{}}`,
		"null list":              `{"data":{"organizations":{"nodes":null}}}`,
		"unusable ID":            `{"data":{"organizations":{"nodes":[{"id":"graph-1","internalNumericId":"1"},{"id":"graph-2","internalNumericId":"two"}]}}}`,
		"missing ID":             `{"data":{"organizations":{"nodes":[{"id":"graph-1"}]}}}`,
	} {
		for _, tc := range triggers {
			t.Run("incomplete answer: "+name+" "+tc.operation, func(t *testing.T) {
				c, _ := serve(t, http.StatusOK, body)

				orgs, err := defaultOrgFetcher(ctx, c, tc.trigger)

				require.Error(t, err)
				require.Nil(t, orgs)
			})
		}
	}

	t.Run("a user in no organizations", func(t *testing.T) {
		c, _ := serve(t, http.StatusOK, `{"data":{"organizations":{"nodes":[]}}}`)

		orgs, err := defaultOrgFetcher(ctx, c, orgCheckPeriodic)

		require.NoError(t, err)
		require.Empty(t, orgs)
	})
}
