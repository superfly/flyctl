package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/logger"
	"github.com/superfly/flyctl/internal/task"
	"github.com/superfly/macaroon"
	"github.com/superfly/macaroon/flyio"
	"github.com/superfly/macaroon/tp"
)

// UserURLCallback is a function that opens a URL in the user's browser. This is
// used for token discharge flows that require user interaction.
type UserURLCallback func(ctx context.Context, url string) error

// MonitorTokens does some housekeeping on the provided tokens. Then, in a
// goroutine, it continues to keep the tokens updated and fresh. The call to
// MonitorTokens will return as soon as the tokens are ready for use and the
// background job will run until the context is cancelled. Token updates include
//   - Keeping the tokens synced with the config file.
//   - Refreshing any expired discharge tokens.
//   - Pruning expired or invalid token.
//   - Fetching macaroons for any organizations the user has been added to.
//   - Pruning tokens for organizations the user is no longer a member of.
//
// The returned function synchronizes file-backed tokens on demand, serialized
// with the background monitor's updates. It performs no API calls. The error
// reports the initial discharge attempt, which may have partially succeeded.
func MonitorTokens(monitorCtx context.Context, t *tokens.Tokens, uucb UserURLCallback) (func() error, error) {
	log := logger.FromContext(monitorCtx)
	file := t.FromFile()
	before := t.Copy()
	m := tokenMonitor{disk: before}
	updated := before.Copy()

	updated1, err := fetchOrgTokens(monitorCtx, updated, orgCheckStartup)
	if err != nil {
		log.Debugf("failed to fetch missing tokens org tokens: %s", err)
	}

	updated2, dischargeErr := refreshDischargeTokens(monitorCtx, updated, uucb, 30*time.Second)
	if dischargeErr != nil {
		log.Debugf("failed to update discharge tokens: %s", dischargeErr)
	}

	if updated1 || updated2 {
		if err := commitTokenRefresh(&m, t, before, updated, file); err != nil {
			log.Debugf("failed to persist updated tokens: %s", err)
		}
	}

	task.FromContext(monitorCtx).Run(func(taskCtx context.Context) {
		taskCtx, cancelTask := context.WithCancelCause(taskCtx)

		var wg sync.WaitGroup

		wg.Add(2)

		if file != "" {
			log.Debugf("monitoring tokens at %s", file)
		} else {
			log.Debug("monitoring tokens in memory")
		}

		go monitorConfigTokenChanges(taskCtx, &m, t, wg.Done)
		go keepConfigTokensFresh(taskCtx, &m, t, uucb, wg.Done)

		// shut down when the task manager is shutting down or when the
		// ctx passed into MonitorTokens is cancelled.
		select {
		case <-taskCtx.Done():
		case <-monitorCtx.Done():
		}

		log.Debug("done monitoring tokens")
		cancelTask(fmt.Errorf("token monitoring stopped: %w", context.Canceled))
		wg.Wait()
	})

	return func() error { return syncConfigTokens(&m, t) }, dischargeErr
}

// monitorConfigTokenChanges watches for token changes in the config file. This can
// happen if a foreground process updates the config file while the agent is
// running.
func monitorConfigTokenChanges(ctx context.Context, m *tokenMonitor, t *tokens.Tokens, done func()) error {
	defer done()

	file := t.FromFile()
	if file == "" {
		return nil
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := syncConfigTokens(m, t); err != nil {
				return err
			}
		}
	}
}

// tokenMonitor remembers the last disk value separately from usable in-memory
// credentials: a failed save must not make an unchanged file undo a refresh.
type tokenMonitor struct {
	sync.Mutex
	disk *tokens.Tokens
}

// syncConfigTokens reads under the monitor lock so an older read cannot
// overwrite a newer sync or interleave with a refresh commit.
func syncConfigTokens(m *tokenMonitor, t *tokens.Tokens) error {
	m.Lock()
	defer m.Unlock()
	file := t.FromFile()
	if file == "" {
		return nil
	}
	current, err := ReadAccessToken(file)
	if err != nil {
		return err
	}
	disk := tokens.ParseFromFile(current, file)
	if !disk.Equal(m.disk) {
		t.Replace(disk)
		m.disk = disk
	}

	return nil
}

// commitTokenRefresh checks both the monitored snapshot and the file before
// committing. The file comparison and write share the config file lock.
func commitTokenRefresh(m *tokenMonitor, t, before, updated *tokens.Tokens, file string) error {
	m.Lock()
	defer m.Unlock()
	if !before.Equal(t) && !preservesCompletedDischarge(before, t, updated) {
		return nil
	}
	var err error
	if file != "" {
		var current *tokens.Tokens
		current, err = setAccessTokenIfUnchanged(file, m.disk, updated)
		if current != nil {
			// Make progress even if the watcher has stopped after a read error.
			updated = current
		}
		if err == nil {
			m.disk = updated.Copy()
		}
	}
	// Keep refreshed credentials usable in memory even if persistence failed.
	t.Replace(updated)

	return err
}

// preservesCompletedDischarge allows a completed authentication to beat a partial
// refresh, but never a change of credentials. All checks are local; no discharge
// request or browser interaction happens while the monitor/config locks are held.
func preservesCompletedDischarge(before, current, updated *tokens.Tokens) bool {
	roots, ok := credentialRoots(before)
	if !ok {
		return false
	}
	for _, other := range []*tokens.Tokens{current, updated} {
		otherRoots, ok := credentialRoots(other)
		if !ok || !slices.Equal(roots, otherRoots) {
			return false
		}
	}
	client := flyio.DischargeClient()
	needs, err := client.NeedsDischarge(current.All())
	if err != nil || !needs {
		return false
	}
	needs, err = client.NeedsDischarge(updated.All())

	return err == nil && !needs
}

// Compare exact permission bytes, not org IDs: scopes and caveats are part of
// credential identity. Discharges may differ; malformed tokens fail closed.
func credentialRoots(t *tokens.Tokens) ([]string, bool) {
	roots := t.GetUserTokens()
	for _, encoded := range t.GetMacaroonTokens() {
		raw, err := macaroon.Parse(encoded)
		if err != nil {
			return nil, false
		}
		for _, token := range raw {
			m, err := macaroon.Decode(token)
			if err != nil {
				return nil, false
			}
			if m.Location == flyio.LocationPermission {
				roots = append(roots, string(token))
			}
		}
	}
	slices.Sort(roots)

	return roots, true
}

const (
	// dischargeRefreshInterval is how often the background monitor refreshes
	// discharge tokens. They expire after 10 minutes, so it has to stay short.
	dischargeRefreshInterval = time.Minute

	// orgCheckInterval is how often the background monitor asks web which
	// organizations the user belongs to. Every command already asks when it
	// starts, and the agent picks that result up from the config file, so the
	// timer only matters to a process that outlives a membership change. The
	// check rarely finds anything to do, and once a minute from every agent it
	// was about half of web's GraphQL database time (superfly/web#5177).
	orgCheckInterval = 15 * time.Minute

	// orgCheckRetryInterval is how soon a failed check is tried again.
	orgCheckRetryInterval = 5 * time.Minute
)

// orgCheckTrigger says why the organization check is running. It only picks
// the name of the GraphQL operation, so web's logs can tell the two apart.
type orgCheckTrigger int

const (
	// orgCheckStartup is the check MonitorTokens makes before it returns.
	orgCheckStartup orgCheckTrigger = iota
	// orgCheckPeriodic is the background monitor's timer.
	orgCheckPeriodic
)

// orgCheckSchedule decides when the background monitor checks organizations
// again.
type orgCheckSchedule struct {
	next time.Time
	// users are the user tokens the last check ran with.
	users  []string
	jitter func(time.Duration) time.Duration
}

// newOrgCheckSchedule starts from a check that has just run for users.
func newOrgCheckSchedule(now time.Time, users []string, jitter func(time.Duration) time.Duration) *orgCheckSchedule {
	s := &orgCheckSchedule{jitter: jitter}
	s.done(now, users, nil)

	return s
}

// due reports whether to check now: the interval has passed, or the user
// tokens aren't the ones last checked, which means a different login.
func (s *orgCheckSchedule) due(now time.Time, users []string) bool {
	return !now.Before(s.next) || !slices.Equal(users, s.users)
}

// done records a check that ran for users. A failed check is retried sooner.
func (s *orgCheckSchedule) done(now time.Time, users []string, err error) {
	s.users = slices.Clone(users)
	if err != nil {
		s.next = now.Add(orgCheckRetryInterval)

		return
	}
	s.next = now.Add(s.jitter(orgCheckInterval))
}

// jitterTenPercent returns d moved by up to 10% either way, so processes
// started together don't keep checking together.
func jitterTenPercent(d time.Duration) time.Duration {
	return d - d/10 + rand.N(d/5+1)
}

// tokenRefreshOps are the remote calls the background monitor makes. Tests
// replace them.
type tokenRefreshOps struct {
	fetchOrgTokens         func(context.Context, *tokens.Tokens, orgCheckTrigger) (bool, error)
	refreshDischargeTokens func(context.Context, *tokens.Tokens, UserURLCallback, time.Duration) (bool, error)
}

// keepConfigTokensFresh periodically updates our tokens and syncs those to the config
// file.
func keepConfigTokensFresh(ctx context.Context, m *tokenMonitor, t *tokens.Tokens, uucb UserURLCallback, done func()) error {
	defer done()

	ops := tokenRefreshOps{fetchOrgTokens: fetchOrgTokens, refreshDischargeTokens: refreshDischargeTokens}

	return refreshTokensPeriodically(ctx, m, t, uucb, ops, jitterTenPercent)
}

// refreshTokensPeriodically refreshes discharge tokens every
// dischargeRefreshInterval and checks organizations every orgCheckInterval.
// The two share one loop so that their results are committed together.
func refreshTokensPeriodically(
	ctx context.Context,
	m *tokenMonitor,
	t *tokens.Tokens,
	uucb UserURLCallback,
	ops tokenRefreshOps,
	jitter func(time.Duration) time.Duration,
) error {
	ticker := time.NewTicker(dischargeRefreshInterval)
	defer ticker.Stop()

	logger := logger.FromContext(ctx)
	file := t.FromFile()
	// MonitorTokens checked organizations just before it started this loop.
	orgCheck := newOrgCheckSchedule(time.Now(), t.GetUserTokens(), jitter)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			beforeUpdate := t.Copy()
			localCopy := beforeUpdate.Copy()

			var updated1 bool
			if users := localCopy.GetUserTokens(); orgCheck.due(time.Now(), users) {
				var err error
				updated1, err = ops.fetchOrgTokens(ctx, localCopy, orgCheckPeriodic)
				if err != nil {
					logger.Debugf("failed to fetch missing org tokens: %s", err)
					// don't continue. might have been partial success
				}
				orgCheck.done(time.Now(), users, err)
			}

			updated2, err := ops.refreshDischargeTokens(ctx, localCopy, uucb, 2*time.Minute)
			if err != nil {
				logger.Debugf("failed to update discharge tokens: %s", err)
				// don't continue. might have been partial success
			}

			if !updated2 && !updated1 {
				continue
			}

			if err := commitTokenRefresh(m, t, beforeUpdate, localCopy, file); err != nil {
				logger.Debugf("failed to persist updated tokens: %s", err)
				// Preserve the existing policy: stop persisting after a write error.
				file = ""
			}
		}
	}
}

// refreshDischargeTokens attempts to refresh any expired discharge tokens. It
// returns true if any tokens were updated, which might be the case even if
// there was an error for other tokens.
//
// Some discharges may require user interaction in the form of opening a URL in
// the user's browser. Set the UserURLCallback package variable if you want to
// support this.
//
// Don't call this when other goroutines might also be accessing it.
func refreshDischargeTokens(ctx context.Context, t *tokens.Tokens, uucb UserURLCallback, advancePrune time.Duration) (bool, error) {
	return doRefreshDischargeTokens(ctx, t, uucb, advancePrune, nonInteractiveDischargeTimeout, InteractiveDischargeTimeout)
}

const (
	// nonInteractiveDischargeTimeout bounds a discharge pass that has no
	// UserURLCallback attached. A third party asking for the user fails
	// immediately in that pass, so everything it can still do is a server round
	// trip and it has no reason to hold the interactive budget.
	nonInteractiveDischargeTimeout = 30 * time.Second

	// InteractiveDischargeTimeout bounds the retry that can send the user to
	// their browser. It is sized for a person opening the URL, possibly on
	// another machine, and working through an identity provider, which is
	// where the whole cost of that pass is. It matches the 15 minutes
	// `fly auth login` waits for approval.
	InteractiveDischargeTimeout = 15 * time.Minute
)

func doRefreshDischargeTokens(
	ctx context.Context,
	t *tokens.Tokens,
	uucb UserURLCallback,
	advancePrune time.Duration,
	nonInteractiveTimeout time.Duration,
	interactiveTimeout time.Duration,
) (bool, error) {
	updateOpts := []tokens.UpdateOption{
		tokens.WithDebugger(logger.FromContext(ctx)),
		tokens.WithAdvancePrune(advancePrune),
		tokens.WithDischargeTimeout(nonInteractiveTimeout),
	}

	// tokens discharged by the first pass have to be reported even if the
	// second one fails: our callers decide whether to persist the tokens from
	// this return value, and dropping it means re-fetching those discharges on
	// every command.
	var updatedParallel bool

	if uucb != nil {
		// Update without UserURLCallback to fetch tokens in parallel.
		updated, err := t.Update(ctx, updateOpts...)
		if err == nil || !errors.Is(err, tp.ErrMissingUserURLCallback) {
			return updated, err
		}
		updatedParallel = updated

		// Retry with UserURLCallback if a third party wants to send the user to
		// their browser. This is the pass with a person in it, and the only one
		// that needs the longer budget; the later option wins.
		updateOpts = append(updateOpts,
			tokens.WithUserURLCallback(uucb),
			tokens.WithDischargeTimeout(interactiveTimeout),
		)
	}

	updated, err := t.Update(ctx, updateOpts...)

	return updated || updatedParallel, err
}

// fetchOrgTokens checks that we macaroons for all orgs the user is a member of.
// It returns true if any new tokens were added, which might be the case even if
// there was an error.
//
// Don't call this when other goroutines might also be accessing t.
func fetchOrgTokens(ctx context.Context, t *tokens.Tokens, trigger orgCheckTrigger) (bool, error) {
	// don't fetch missing org tokens if tokens came from environment var
	if t.FromFile() == "" {
		return false, nil
	}

	fetchOrgs := func(ctx context.Context, c flyutil.Client) (map[uint64]string, error) {
		return defaultOrgFetcher(ctx, c, trigger)
	}

	return doFetchOrgTokens(ctx, t, fetchOrgs, defaultTokenMinter)
}

func doFetchOrgTokens(ctx context.Context, t *tokens.Tokens, fetchOrgs orgFetcher, mintToken tokenMinter) (bool, error) {
	macToks := t.GetMacaroonTokens()

	if len(macToks) == 0 || len(t.GetUserTokens()) == 0 {
		return false, nil
	}

	c := flyutil.NewClientFromOptions(ctx, fly.ClientOptions{Tokens: t.UserTokenOnly()})

	graphIDByNumericID, err := fetchOrgs(ctx, c)
	if err != nil {
		return false, err
	}

	log := logger.FromContext(ctx)

	tokOIDS := make(map[uint64]bool, len(macToks))
	macToks = slices.DeleteFunc(macToks, func(tok string) bool {
		toks, err := macaroon.Parse(tok)
		if err != nil {
			log.Debugf("pruning token: failed to parse macaroon: %v", err)

			return true
		}

		permMacs, _, _, _, err := macaroon.FindPermissionAndDischargeTokens(toks, flyio.LocationPermission)
		if err != nil {
			log.Debugf("pruning token: failed to find permission token: %v", err)

			return true
		}

		// discharge token?
		if len(permMacs) != 1 {
			return false
		}

		oid, err := flyio.OrganizationScope(&permMacs[0].UnsafeCaveats)
		if err != nil {
			log.Debugf("pruning token: failed to calculate org scope: %v", err)

			return true
		}

		if _, hasOrg := graphIDByNumericID[oid]; !hasOrg {
			log.Debug("pruning token: not in org")

			return true
		}

		tokOIDS[oid] = true

		return false
	})

	// find missing orgs by deleting the ones we found
	for oid := range tokOIDS {
		delete(graphIDByNumericID, oid)
	}

	var (
		wg     sync.WaitGroup
		wgErr  error
		wgLock sync.Mutex
	)

	addErr := func(err error) {
		wgLock.Lock()
		defer wgLock.Unlock()
		wgErr = errors.Join(wgErr, err)
	}
	addMac := func(m string) {
		wgLock.Lock()
		defer wgLock.Unlock()
		macToks = append(macToks, m)
	}
	for graphID := range maps.Values(graphIDByNumericID) {

		wg.Go(func() {

			log.Debugf("fetching macaroons for org %s", graphID)
			newToksStr, err := mintToken(ctx, c, graphID)
			if err != nil {
				addErr(fmt.Errorf("failed to get macaroons for org %s: %w", graphID, err))

				return
			}

			newToks, err := macaroon.Parse(newToksStr)
			if err != nil {
				addErr(fmt.Errorf("bad macaroons for org %s: %w", graphID, err))

				return
			}

			for _, newTok := range newToks {
				m, err := macaroon.Decode(newTok)
				if err != nil {
					addErr(fmt.Errorf("bad macaroon for org %s: %w", graphID, err))

					return
				}

				mStr, err := m.String()
				if err != nil {
					addErr(fmt.Errorf("failed encoding macaroon for org %s: %w", graphID, err))

					return
				}

				addMac(mStr)
			}
		})
	}
	wg.Wait()

	if slices.Equal(macToks, t.GetMacaroonTokens()) {
		return false, wgErr
	}

	t.ReplaceMacaroonTokens(macToks)

	return true, wgErr
}

// orgFetcher allows us to stub out gql calls in tests
type orgFetcher func(context.Context, flyutil.Client) (map[uint64]string, error)

// defaultOrgFetcher asks web for the organizations the user belongs to. It
// requests only the two IDs it uses: web runs extra queries per organization
// for fields like viewerRole and billable.
func defaultOrgFetcher(ctx context.Context, c flyutil.Client, trigger orgCheckTrigger) (map[uint64]string, error) {
	graphIDByNumericID := map[uint64]string{}
	add := func(graphID, numericID string) {
		if uintID, err := strconv.ParseUint(numericID, 10, 64); err == nil {
			graphIDByNumericID[uintID] = graphID
		}
	}

	if trigger == orgCheckPeriodic {
		resp, err := gql.OrgTokenCheckPeriodic(ctx, c.GenqClient())
		if err != nil {
			return nil, err
		}
		for _, org := range resp.Organizations.Nodes {
			add(org.Id, org.InternalNumericId)
		}

		return graphIDByNumericID, nil
	}

	resp, err := gql.OrgTokenCheckStartup(ctx, c.GenqClient())
	if err != nil {
		return nil, err
	}
	for _, org := range resp.Organizations.Nodes {
		add(org.Id, org.InternalNumericId)
	}

	return graphIDByNumericID, nil
}

// tokenMinter allows us to stub out gql calls in tests
type tokenMinter func(context.Context, flyutil.Client, string) (string, error)

func defaultTokenMinter(ctx context.Context, c flyutil.Client, id string) (string, error) {
	resp, err := gql.CreateLimitedAccessToken(ctx, c.GenqClient(), "flyctl", id, "deploy_organization", &gql.LimitedAccessTokenOptions{}, "10m")
	if err != nil {
		return "", err
	}

	return resp.CreateLimitedAccessToken.GetLimitedAccessToken().TokenHeader, nil
}

// ScopedIDs returns the internal numeric IDs of the organization and app that t
// is scoped to, or zero for either one that can't be determined. It only reads
// the caveats of the tokens we already hold, so it costs nothing and works even
// for commands that never look an app up.
//
// Tokens hold one macaroon per organization the user belongs to (see
// fetchOrgTokens), so an organization is only reported when every permission
// macaroon agrees on the same one. In practice that means single-org tokens --
// CI and deploy tokens, and users who belong to a single organization -- report
// an org, and a token spanning several organizations reports none rather than
// guessing. Apps are only reported by tokens narrowed to exactly one app.
func ScopedIDs(t *tokens.Tokens) (orgID, appID uint64) {
	if t == nil {
		return 0, 0
	}

	orgIDs := map[uint64]struct{}{}
	appIDs := map[uint64]struct{}{}

	for _, tok := range t.GetMacaroonTokens() {
		parsed, err := macaroon.Parse(tok)
		if err != nil {
			continue
		}

		// Discharge tokens carry no permissions, and a bundle covering several
		// orgs can't be attributed to one, so both yield no permission macaroon
		// to read here.
		permMacs, _, _, _, err := macaroon.FindPermissionAndDischargeTokens(parsed, flyio.LocationPermission)
		if err != nil || len(permMacs) != 1 {
			continue
		}

		if oid, err := flyio.OrganizationScope(&permMacs[0].UnsafeCaveats); err == nil {
			orgIDs[oid] = struct{}{}
		}

		// AppScope returns nil when the token is valid for every app, so a
		// non-empty result always names specific apps.
		for _, aid := range flyio.AppScope(&permMacs[0].UnsafeCaveats) {
			appIDs[aid] = struct{}{}
		}
	}

	return onlyID(orgIDs), onlyID(appIDs)
}

// onlyID returns the single ID in ids, or zero if it holds anything else.
func onlyID(ids map[uint64]struct{}) uint64 {
	if len(ids) != 1 {
		return 0
	}

	for id := range ids {
		return id
	}

	return 0
}
