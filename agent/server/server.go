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
	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/agent"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/wg"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/internal/sentry"
	"github.com/superfly/flyctl/wireguard"
)

type Options struct {
	Socket     string
	Logger     *log.Logger
	Client     *api.Client
	Background bool
	ConfigFile string
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

	err = (&server{
		Options:       opt,
		listener:      l,
		currentChange: latestChangeAt,
		tunnels:       make(map[string]*wg.Tunnel),
	}).serve(ctx, l)

	return
}

type bindError struct{ error }

func (be bindError) Unwrap() error { return be.error }

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

	if l, err = net.Listen("unix", socket); err != nil {
		err = fmt.Errorf("failed binding: %w", err)
	}

	return
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

type server struct {
	Options

	listener net.Listener

	mu            sync.Mutex
	currentChange time.Time
	tunnels       map[string]*wg.Tunnel
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

func (s *server) buildTunnel(org *api.Organization, recycle bool) (tunnel *wg.Tunnel, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if tunnel = s.tunnels[org.Slug]; tunnel != nil && !recycle {
		// tunnel already exists
		return
	}

	var state *wg.WireGuardState
	if state, err = wireguard.StateForOrg(s.Client, org, "", "", recycle); err != nil {
		return
	}

	// WIP: can't stay this way, need something more clever than this
	if os.Getenv("WSWG") != "" || viper.GetBool(flyctl.ConfigWireGuardWebsockets) {
		if tunnel, err = wg.ConnectWS(context.Background(), state); err != nil {
			return
		}
	} else {
		if tunnel, err = wg.Connect(context.Background(), state); err != nil {
			return
		}
	}

	s.tunnels[org.Slug] = tunnel

	return
}

func (s *server) fetchInstances(ctx context.Context, tunnel *wg.Tunnel, app string) (*agent.Instances, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
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

	for _, region := range strings.Split(regions, ",") {
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

func (s *server) tunnelFor(slug string) *wg.Tunnel {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.tunnels[slug]
}

func (s *server) probeTunnel(ctx context.Context, slug string) (err error) {
	tunnel := s.tunnelFor(slug)
	if tunnel == nil {
		err = agent.ErrTunnelUnavailable

		return
	}

	s.printf("probing %q ...", slug)

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var results []string
	switch results, err = tunnel.LookupTXT(ctx, "_apps.internal"); {
	case err != nil:
		err = fmt.Errorf("failed probing %q: %w", slug, err)
	case len(results) == 0:
		s.printf("%q probed.", slug)
	default:
		s.printf("%q probed: %s", slug, strings.Join(results, ", "))
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
		if peers[slug] == nil {
			delete(s.tunnels, slug)

			s.printf("no peer for %s in config - closing tunnel ...", slug)

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

		if err := wireguard.PruneInvalidPeers(ctx, s.Client); err != nil {
			s.printf("failed pruning invalid peers: %v", err)
		}

		if err := s.validateTunnels(); err != nil {
			s.printf("failed validating tunnels: %v", err)
		}

		s.print("validated wireguard peers")
	}
}

func (s *server) print(v ...interface{}) {
	s.Logger.Print(v...)
}

func (s *server) printf(format string, v ...interface{}) {
	s.Logger.Printf(format, v...)
}
