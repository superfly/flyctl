package config

import (
	"context"
	"errors"
	"fmt"
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
	"golang.org/x/exp/maps"
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
func MonitorTokens(monitorCtx context.Context, t *tokens.Tokens, uucb UserURLCallback) {
	log := logger.FromContext(monitorCtx)
	file := t.FromFile()

	updated1, err := fetchOrgTokens(monitorCtx, t)
	if err != nil {
		log.Debugf("failed to fetch missing tokens org tokens: %s", err)
	}

	updated2, err := refreshDischargeTokens(monitorCtx, t, uucb)
	if err != nil {
		log.Debugf("failed to update discharge tokens: %s", err)
	}

	if file != "" && (updated1 || updated2) {
		if err := SetAccessToken(file, t.All()); err != nil {
			log.Debugf("failed to persist updated tokens: %s", err)
		}
	}

	task.FromContext(monitorCtx).Run(func(taskCtx context.Context) {
		taskCtx, cancelTask := context.WithCancel(taskCtx)

		var m sync.Mutex
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
		cancelTask()
		wg.Wait()
	})
}

// monitorConfigTokenChanges watches for token changes in the config file. This can
// happen if a foreground process updates the config file while the agent is
// running.
func monitorConfigTokenChanges(ctx context.Context, m *sync.Mutex, t *tokens.Tokens, done func()) error {
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
			currentStr, err := ReadAccessToken(file)
			if err != nil {
				return err
			}

			current := tokens.ParseFromFile(currentStr, file)

			m.Lock()
			t.Replace(current)
			m.Unlock()
		}
	}
}

// keepConfigTokensFresh periodically updates our tokens and syncs those to the config
// file.
func keepConfigTokensFresh(ctx context.Context, m *sync.Mutex, t *tokens.Tokens, uucb UserURLCallback, done func()) error {
	defer done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	logger := logger.FromContext(ctx)
	file := t.FromFile()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			localCopy := t.Copy()
			beforeUpdate := t.Copy()

			updated1, err := fetchOrgTokens(ctx, localCopy)
			if err != nil {
				logger.Debugf("failed to fetch missing org tokens: %s", err)
				// don't continue. might have been partial success
			}

			updated2, err := refreshDischargeTokens(ctx, localCopy, uucb)
			if err != nil {
				logger.Debugf("failed to update discharge tokens: %s", err)
				// don't continue. might have been partial success
			}

			if !updated2 && !updated1 {
				continue
			}

			m.Lock()
			// don't clobber config file if it changed out from under us. the
			// consequences of a race here (agent and foreground command both
			// fetching updates simultaneously) are low, so don't bother with an
			// extra lock file.
			if beforeUpdate.Equal(t) {
				t.Replace(localCopy)

				if file != "" {
					if err := SetAccessToken(file, t.All()); err != nil {
						logger.Debugf("failed to persist updated tokens: %s", err)

						// don't try again if we fail to write once
						file = ""
					}
				}
			}
			m.Unlock()
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
// Don't call this when other goroutines might also be accessing t.
func refreshDischargeTokens(ctx context.Context, t *tokens.Tokens, uucb UserURLCallback) (bool, error) {
	updateOpts := []tokens.UpdateOption{tokens.WithDebugger(logger.FromContext(ctx))}

	if uucb != nil {
		updateOpts = append(updateOpts, tokens.WithUserURLCallback(uucb))
	}

	return t.Update(ctx, updateOpts...)
}

// fetchOrgTokens checks that we macaroons for all orgs the user is a member of.
// It returns true if any new tokens were added, which might be the case even if
// there was an error.
//
// Don't call this when other goroutines might also be accessing t.
func fetchOrgTokens(ctx context.Context, t *tokens.Tokens) (bool, error) {
	// don't fetch missing org tokens if tokens came from environment var
	if t.FromFile() == "" {
		return false, nil
	}

	return doFetchOrgTokens(ctx, t, defaultOrgFetcher, defaultTokenMinter)
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
	for _, graphID := range maps.Values(graphIDByNumericID) {
		graphID := graphID

		wg.Add(1)
		go func() {
			defer wg.Done()

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
		}()
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

func defaultOrgFetcher(ctx context.Context, c flyutil.Client) (map[uint64]string, error) {
	orgs, err := c.GetOrganizations(ctx)
	if err != nil {
		return nil, err
	}

	graphIDByNumericID := make(map[uint64]string, len(orgs))
	for _, org := range orgs {
		if uintID, err := strconv.ParseUint(org.InternalNumericID, 10, 64); err == nil {
			graphIDByNumericID[uintID] = org.ID
		}
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
