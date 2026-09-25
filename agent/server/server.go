package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/azazeal/pause"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/env"
	"github.com/superfly/flyctl/internal/flyutil"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/metrics/synthetics"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/wg"
	"golang.org/x/sync/errgroup"
)

type Options struct {
	Socket           string
	Logger           *log.Logger
	Background       bool
	ConfigFile       string
	ConfigWebsockets bool

	// TokenMode makes the agent build tunnels by authenticating to
	// TokenGateway with the session's macaroons instead of registering
	// peers through the API.
	TokenMode    bool
	TokenGateway string
}

func Run(ctx context.Context, opt Options) (err error) {
	var l net.Listener
	if l, err = bind(opt.Socket); err != nil {
		opt.Logger.Print(err)

		return
	}
	// serve will close the listener

	var latestChangeAt time.Time
	if latestChangeAt, err = latestChange(opt.ConfigFile); err != nil {
		_ = l.Close()

		opt.Logger.Print(err)

		return
	}

	toks := config.Tokens(ctx)

	monitorCtx, cancelMonitorCause := context.WithCancelCause(ctx)

	cancelMonitor := func() { cancelMonitorCause(fmt.Errorf("agent token monitoring stopped: %w", context.Canceled)) }
	config.MonitorTokens(monitorCtx, toks, nil)

	synthetics.StartSyntheticsMonitoringAgent(ctx)

	err = (&server{
		Options:               opt,
		listener:              l,
		runCtx:                ctx,
		currentChange:         latestChangeAt,
		tunnels:               make(map[tunnelKey]*wg.Tunnel),
		slugAliases:           make(map[string]string),
		tokens:                toks,
		cancelTokenMonitoring: cancelMonitor,
	}).serve(ctx, l)

	return
}

type bindError struct{ error }

func (be bindError) Unwrap() error { return be.error }

// maxUnixSocketPath is the portable limit on unix socket paths (sun_path is
// 108 bytes on Linux, 104 on the BSDs and macOS, NUL included).
const maxUnixSocketPath = 103

func bindUnixSocket(socket string) (net.Listener, error) {
	if len(socket) > maxUnixSocketPath {
		return nil, fmt.Errorf("socket path %q is %d bytes, over the %d-byte unix socket limit: set %s (or TMPDIR, in isolated mode) to a shorter path", socket, len(socket), maxUnixSocketPath, agent.SocketPathEnvKey)
	}

	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("failed binding: %w", err)
	}

	return l, nil
}

func bind(socket string) (l net.Listener, err error) {
	defer func() {
		if err != nil {
			sentry.CaptureException(bindError{err})
		}
	}()

	if err = removeSocket(socket); err != nil {
		err = fmt.Errorf("failed removing existing socket: %w", err)

		return
	}

	return bindSocket(socket)
}

func latestChange(path string) (at time.Time, err error) {
	var info os.FileInfo
	switch info, err = os.Stat(path); err {
	default:
		err = fmt.Errorf("can't stat config file: %w", err)
	case nil:
		at = info.ModTime()
	}

	return
}

type tunnelKey struct {
	orgSlug     string
	networkName string
}

func (tk tunnelKey) String() string {
	if tk.networkName == "" {
		return tk.orgSlug
	}

	return tk.orgSlug + "/" + tk.networkName
}

type server struct {
	Options

	listener net.Listener

	runCtx        context.Context
	mu            sync.Mutex
	currentChange time.Time
	tunnels       map[tunnelKey]*wg.Tunnel
	// slugAliases maps every slug a client has used for an organization to
	// the slug tunnels are keyed by. Clients may name the personal org by
	// its raw slug (as Flaps does) or by the "personal" alias (as web does).
	slugAliases map[string]string

	// tokMu guards the two below, separately from mu so that code running
	// under mu (tunnel construction, which presents tokens) can read them.
	tokMu                 sync.RWMutex
	tokens                *tokens.Tokens
	cancelTokenMonitoring func()
}

type terminateError struct{ error }

func (te terminateError) Unwrap() error { return te.error }

var errShutdown = errors.New("shutdown")

func (s *server) serve(parent context.Context, l net.Listener) (err error) {
	eg, ctx := errgroup.WithContext(parent)

	eg.Go(func() error {
		<-ctx.Done()

		if err := l.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.printf("failed closing listener: %v", err)
		}

		return errShutdown
	})

	eg.Go(func() error {
		s.clean(ctx)

		return nil
	})

	eg.Go(func() (err error) {
		s.printf("OK %d", os.Getpid())
		defer s.print("QUIT")

		var sID uint64

		for {
			var conn net.Conn
			if conn, err = s.listener.Accept(); err == nil {
				eg.Go(func() error {
					runSession(ctx, s, conn, id(atomic.AddUint64(&sID, 1)))

					return nil
				})

				continue
			}

			switch ne, ok := err.(net.Error); {
			case ok && ne.Temporary():
				continue
			case errors.Is(err, net.ErrClosed):
				err = errShutdown

				s.print("shutting down ...")
			default:
				s.printf("encountered terminal error: %v", err)
			}

			return
		}
	})

	switch err = eg.Wait(); {
	default:
		sentry.CaptureException(terminateError{err})
	case errors.Is(err, errShutdown):
		err = nil // we initiated the shutdown
	}

	return
}

func (s *server) shutdown() {
	_ = s.listener.Close()
}

func (s *server) checkForConfigChange() (err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var at time.Time
	if at, err = latestChange(s.ConfigFile); err != nil {
		err = fmt.Errorf("can't stat config file: %w", err)

		return
	}

	if at.After(s.currentChange) {
		s.currentChange = at

		if err = s.validateTunnelsUnlocked(); err != nil {
			err = fmt.Errorf("can't validate peers: %w", err)
		} else {
			s.printf("config change at: %v", s.currentChange)
		}
	}

	return
}

// buildTunnel returns the tunnel for org/network, building it if needed.
// sessionToks are the tokens the client sent, nil if none; see tokensFor.
func (s *server) buildTunnel(ctx context.Context, org *fly.Organization, reestablish bool, network string, client flyutil.Client, sessionToks *tokens.Tokens) (tunnel *wg.Tunnel, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tk := tunnelKey{orgSlug: org.Slug, networkName: network}

	// not checking the region is intentional, it's static during the lifetime of the agent
	if tunnel = s.tunnels[tk]; tunnel != nil && !reestablish {
		// tunnel already exists
		return
	}

	var transport string
	if s.TokenMode {
		transport = "token"

		if tunnel, err = s.buildTokenTunnel(ctx, org, network, sessionToks); err != nil {
			s.reportTokenFailure(tk, err, nil)

			return
		}
	} else {
		var state *wg.WireGuardState
		if state, err = wireguard.StateForOrg(ctx, client, org.ID, org.Slug, os.Getenv("FLY_AGENT_WG_REGION"), "", reestablish, network); err != nil {
			return
		}

		transport = "udp"
		if env.IsCI() || os.Getenv("WSWG") != "" || s.ConfigWebsockets {
			transport = "websocket"

			if tunnel, err = wg.ConnectWS(ctx, state); err != nil {
				return
			}
		} else {
			if tunnel, err = wg.Connect(ctx, state); err != nil {
				return
			}
		}
	}

	// use the agent's run context: the session context may be gone before the
	// send completes
	metrics.AgentWireGuardTransport(s.runCtx, transport)

	if old := s.tunnels[tk]; old != nil {
		s.printf("replacing tunnel for %s", tk)

		if err := old.Close(); err != nil {
			s.printf("failed closing replaced tunnel: %v", err)
		}
	}

	s.tunnels[tk] = tunnel

	if tunnel.Ephemeral() {
		go s.watchTunnel(tk, tunnel)
	}

	return
}

// buildTokenTunnel connects to the token gateway with a fresh keypair. The
// gateway allocates the peer inline and keeps it only as long as the token
// verifies, so nothing is registered with the API or written to the config
// file. Callers must hold s.mu.
func (s *server) buildTokenTunnel(ctx context.Context, org *fly.Organization, network string, sessionToks *tokens.Tokens) (*wg.Tunnel, error) {
	if s.tokensFor(sessionToks).MacaroonsOnly().Empty() {
		return nil, errors.New("access token is too old, please reauthenticate")
	}

	// the gateway looks orgs up by their real slug; "personal" is an API alias
	slug := org.RawSlug
	if slug == "" {
		slug = org.Slug
	}

	pubkey, privkey := wireguard.C25519pair()

	state := &wg.WireGuardState{
		Org:          org.Slug,
		Name:         wg.TokenPeerName(pubkey),
		LocalPublic:  pubkey,
		LocalPrivate: privkey,
	}

	s.printf("connecting to token gateway %s for %s (network %q) as %s", s.TokenGateway, slug, network, state.Name)

	tunnel, err := wg.ConnectToken(ctx, state, s.TokenGateway, &wg.TokenAuth{
		// resolved on every (re-)connection, so the tunnel follows the
		// agent's current, monitored tokens even after they're replaced
		Token:       func() string { return s.tokensFor(sessionToks).MacaroonsOnly().FlapsHeader() },
		OrgSlug:     slug,
		NetworkName: network,
	})
	if err != nil {
		return nil, fmt.Errorf("token-mode WireGuard via %s: %w", s.TokenGateway, err)
	}

	if prov := tunnel.Provision(); prov != nil {
		s.printf("token gateway provisioned %s: peer %s, dns %s, expires %s", state.Name, prov.PeerIP, prov.DNS, prov.ExpiresAt.Format(time.RFC3339))
	}

	return tunnel, nil
}

// reportTokenFailure records a token-mode establish failure or tunnel death
// in metrics, and in Sentry when it's not an expected outcome.
func (s *server) reportTokenFailure(tk tunnelKey, err error, prov *wg.TokenProvision) {
	reason := failureReason(err)

	// the agent's run context: the session's may be gone already
	metrics.AgentWireGuardTransportFailure(s.runCtx, "token", reason)

	if !unexpectedTokenFailure(reason, prov, time.Now()) {
		return
	}

	opts := []sentry.CaptureOption{
		sentry.WithTag("feature", "agent-wireguard-token"),
		sentry.WithTag("gateway", s.TokenGateway),
		sentry.WithTag("reason", reason),
		sentry.WithContexts(map[string]sentry.Context{
			"organization": map[string]any{
				"slug":    tk.orgSlug,
				"network": tk.networkName,
			},
		}),
	}
	if prov != nil {
		opts = append(opts, sentry.WithContext("provision", map[string]any{
			"peer_ip":    prov.PeerIP,
			"expires_at": prov.ExpiresAt.Format(time.RFC3339),
		}))
	}

	sentry.CaptureException(err, opts...)
}

// watchTunnel forgets a token-mode tunnel once the gateway has rejected it
// for good (token expired or revoked), so the next establish builds a fresh
// one instead of clients hanging on a dead tunnel.
func (s *server) watchTunnel(tk tunnelKey, tunnel *wg.Tunnel) {
	<-tunnel.Done()

	err := tunnel.Err()
	if err == nil {
		return // closed by us
	}

	s.printf("tunnel for %s died: %v", tk, err)
	s.reportTokenFailure(tk, err, tunnel.Provision())

	s.mu.Lock()
	if s.tunnels[tk] == tunnel {
		delete(s.tunnels, tk)
	}
	s.mu.Unlock()

	if err := tunnel.Close(); err != nil {
		s.printf("failed closing dead tunnel: %v", err)
	}
}

func (s *server) fetchInstances(ctx context.Context, tunnel *wg.Tunnel, app string) (*agent.Instances, error) {
	ctx, cancel := context.WithTimeoutCause(ctx, 30*time.Second, fmt.Errorf("fetching agent instances: %w", context.DeadlineExceeded))
	defer cancel()

	regionsv, err := tunnel.LookupTXT(ctx, fmt.Sprintf("regions.%s.internal", app))
	if err != nil {
		return nil, fmt.Errorf("look up regions for %s: %w", app, err)
	}

	var regions string

	if len(regionsv) > 0 {
		regions = strings.Trim(regionsv[0], " \t")
	}

	if regions == "" {
		return nil, fmt.Errorf("can't find deployed regions for %s", app)
	}

	ret := &agent.Instances{}

	for region := range strings.SplitSeq(regions, ",") {
		name := fmt.Sprintf("%s.%s.internal", region, app)
		addrs, err := tunnel.LookupAAAA(ctx, name)
		if err != nil {
			s.printf("can't lookup records for %s: %s", name, err)

			continue
		}

		if len(addrs) == 1 {
			ret.Labels = append(ret.Labels, name)
			ret.Addresses = append(ret.Addresses, addrs[0].String())

			continue
		}

		for _, addr := range addrs {
			ret.Labels = append(ret.Labels, fmt.Sprintf("%s (%s)", region, addr))
			ret.Addresses = append(ret.Addresses, addr.String())
		}
	}

	return ret, nil
}

// rememberSlug records that slug names the organization whose tunnels are
// keyed by canonical.
func (s *server) rememberSlug(slug, canonical string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.slugAliases[slug] = canonical
}

// canonicalSlugUnlocked returns the slug tunnels are keyed by for a slug a
// client sent. Callers must hold s.mu.
func (s *server) canonicalSlugUnlocked(slug string) string {
	if canonical, ok := s.slugAliases[slug]; ok {
		return canonical
	}

	return slug
}

func (s *server) tunnelFor(slug, network string) *wg.Tunnel {
	s.mu.Lock()
	defer s.mu.Unlock()

	tk := tunnelKey{orgSlug: s.canonicalSlugUnlocked(slug), networkName: network}

	return s.tunnels[tk]
}

func (s *server) probeTunnel(ctx context.Context, slug, network string) (err error) {
	tunnel := s.tunnelFor(slug, network)
	if tunnel == nil {
		err = agent.ErrTunnelUnavailable

		return
	}

	s.printf("probing %q ...", slug)

	ctx, cancel := context.WithTimeoutCause(ctx, 5*time.Second, fmt.Errorf("probing WireGuard tunnel: %w", context.DeadlineExceeded))
	defer cancel()

	var results []net.IP
	switch results, err = tunnel.LookupAAAA(ctx, "_api.internal"); {
	case err != nil:
		// anytime you change the error message here, you need to update https://github.com/superfly/flyctl/blob/df7529f6da985a662853ffc7003f57ee3c9d8e42/internal/build/imgsrc/docker.go#L370
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("timed out (%w)", err)
		}
		err = fmt.Errorf("Error contacting Fly.io API when probing %q: %w", slug, err)
	case len(results) == 0:
		s.printf("%q probed.", slug)
	default:
		s.printf("%q probed: %s", slug, results[0])
	}

	return
}

func (s *server) validateTunnels() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.validateTunnelsUnlocked()
}

func (s *server) validateTunnelsUnlocked() error {
	peers, err := wireguard.GetWireGuardState()
	if err != nil {
		return err
	}

	for slug, tunnel := range s.tunnels {
		if tunnel.Ephemeral() {
			// provisioned inline by the token gateway; not in the config file
			continue
		}

		sk := slug.orgSlug
		if slug.networkName != "" {
			sk = fmt.Sprintf("%s-%s", sk, slug.networkName)
		}

		s.printf("%s, %+v", sk, peers)

		if peers[sk] == nil {
			delete(s.tunnels, slug)

			s.printf("no peer for '%s' in config - closing tunnel ...", slug)

			if err := tunnel.Close(); err != nil {
				s.printf("failed closing tunnel: %v", err)
			}

		}
	}

	return nil
}

func (s *server) clean(ctx context.Context) {
	for {
		if pause.For(ctx, 2*time.Minute); ctx.Err() != nil {
			break
		}

		// token-mode peers aren't registered with the API; nothing to prune
		if !s.TokenMode {
			if err := wireguard.PruneInvalidPeers(ctx, s.GetClient(ctx)); err != nil {
				s.printf("failed pruning invalid peers: %v", err)
			}
		}

		if err := s.validateTunnels(); err != nil {
			s.printf("failed validating tunnels: %v", err)
		}

		s.print("validated wireguard peers")
	}
}

// GetClient returns an API client that uses the server's tokens. Sessions may
// have their own tokens, so should use session.getClient instead.
func (s *server) GetClient(ctx context.Context) flyutil.Client {
	return flyutil.NewClientFromOptions(ctx, fly.ClientOptions{Tokens: s.GetTokens()})
}

// GetTokens returns the server's tokens. Sessions may have their own, so
// should use session.getTokens instead.
func (s *server) GetTokens() *tokens.Tokens {
	s.tokMu.RLock()
	defer s.tokMu.RUnlock()

	return s.tokens
}

// tokensFor returns the tokens to act with on behalf of a client that sent
// sessionToks (nil if it sent none): the client's, except that a client
// reading the same config file the server monitors gets the server's
// current copy. The client's is a one-shot snapshot, while the server's is
// kept fresh (refreshed discharges, new org tokens, replacement via
// UpdateTokensFromClient) for as long as a tunnel built from it lives.
func (s *server) tokensFor(sessionToks *tokens.Tokens) *tokens.Tokens {
	cur := s.GetTokens()

	if sessionToks == nil {
		return cur
	}

	if file := sessionToks.FromFile(); file != "" && file == cur.FromFile() {
		return cur
	}

	return sessionToks
}

// UpdateTokensFromClient replaces the server's tokens with those from the
// client if the new ones seem better. Specifically, if the agent was started
// with `FLY_API_TOKEN`, but a later client is using tokens form a config file.
func (s *server) UpdateTokensFromClient(t *tokens.Tokens) {
	s.tokMu.Lock()
	defer s.tokMu.Unlock()

	if s.tokens.FromFile() != "" || t.FromFile() == "" {
		return
	}

	s.print("received new tokens from client")

	s.cancelTokenMonitoring()

	monitorCtx, cancelMonitorCause := context.WithCancelCause(s.runCtx)

	cancelMonitor := func() { cancelMonitorCause(fmt.Errorf("agent token monitoring replaced: %w", context.Canceled)) }
	config.MonitorTokens(monitorCtx, t, nil)

	s.tokens = t
	s.cancelTokenMonitoring = cancelMonitor
}

func (s *server) print(v ...any) {
	s.Logger.Print(v...)
}

func (s *server) printf(format string, v ...any) {
	s.Logger.Printf(format, v...)
}
