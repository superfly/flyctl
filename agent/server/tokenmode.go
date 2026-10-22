package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/superfly/flyctl/gql"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/launchdarkly"
)

// tokenModeFlagTimeout bounds the user lookup plus flag fetch that precede
// building a tunnel, so an unreachable API or LaunchDarkly delays the tunnel
// by at most this much before the local config decides.
const tokenModeFlagTimeout = 5 * time.Second

// tokenModeFor decides whether the tunnel about to be built should go
// through the token gateway: yes when the wireguard-token-mode feature flag
// is on for the user client authenticates as, otherwise whatever the local
// config said when the agent started (s.TokenMode).
//
// The flag is evaluated afresh on every call so a rollout reaches agents
// that have been running for days. Whenever it can't be evaluated (the user
// lookup fails, the token identifies no user, LaunchDarkly is unreachable)
// the local config decides, as if the flag were off.
func (s *server) tokenModeFor(ctx context.Context, client flyutil.Client) bool {
	on, err := s.tokenModeFlagFor(ctx, client)
	if err != nil {
		s.printf("%s flag: %v; using local config (token mode %t)", launchdarkly.WireGuardTokenModeFlag, err, s.TokenMode)

		return s.TokenMode
	}

	if on {
		s.print("token mode enabled by feature flag")

		return true
	}

	return s.TokenMode
}

// tokenModeFlagFor evaluates the wireguard-token-mode flag for the user
// client authenticates as.
func (s *server) tokenModeFlagFor(ctx context.Context, client flyutil.Client) (bool, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, tokenModeFlagTimeout, fmt.Errorf("evaluating %s flag: %w", launchdarkly.WireGuardTokenModeFlag, context.DeadlineExceeded))
	defer cancel()

	userID, err := currentUserID(ctx, client)
	if err != nil {
		return false, err
	}

	evaluate := s.tokenModeFlag
	if evaluate == nil {
		evaluate = s.evaluateTokenModeFlag
	}

	on, err := evaluate(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("evaluating for user %d: %w", userID, err)
	}

	return on, nil
}

// currentUserID returns the numeric id of the user client authenticates as:
// the one feature flags target, not the opaque GraphQL id.
func currentUserID(ctx context.Context, client flyutil.Client) (int, error) {
	resp, err := gql.GetCurrentUserNumericID(ctx, client.GenqClient())
	if err != nil {
		return 0, fmt.Errorf("looking up current user: %w", err)
	}

	// a macaroon-only session authenticates as a token, not a user: nothing
	// for a per-user flag to target
	user, ok := resp.Viewer.(*gql.GetCurrentUserNumericIDViewerUser)
	if !ok {
		return 0, fmt.Errorf("current viewer is a %T, not a user", resp.Viewer)
	}

	return user.InternalNumericId, nil
}

// flagClients holds one feature-flag client per user id, so that repeated
// establishes for the same user refresh a client instead of building one.
type flagClients struct {
	mu     sync.Mutex
	byUser map[int]*launchdarkly.Client
}

func (fc *flagClients) get(userID int) *launchdarkly.Client {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	if c := fc.byUser[userID]; c != nil {
		return c
	}

	if fc.byUser == nil {
		fc.byUser = make(map[int]*launchdarkly.Client)
	}

	c := launchdarkly.NewUserClient(userID)
	fc.byUser[userID] = c

	return c
}

// evaluateTokenModeFlag is the default tokenModeFlag: it asks LaunchDarkly
// for the user's current flags and reports wireguard-token-mode.
func (s *server) evaluateTokenModeFlag(ctx context.Context, userID int) (bool, error) {
	c := s.flagClients.get(userID)

	if err := c.Refresh(ctx); err != nil {
		return false, err
	}

	return c.WireGuardTokenModeEnabled(), nil
}
