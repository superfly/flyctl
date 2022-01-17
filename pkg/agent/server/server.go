package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/azazeal/pause"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/agent/internal/proto"
	"github.com/superfly/flyctl/pkg/wg"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/terminal"
)

func Run(ctx context.Context, logger *log.Logger, apiClient *api.Client, background bool) (err error) {
	var l net.Listener
	if l, err = bind(); err != nil {
		logger.Print(err)

		return
	}

	var latestChangeAt time.Time
	if latestChangeAt, err = latestChange(); err != nil {
		logger.Print(err)

		return
	}

	s := &server{
		logger:        logger,
		listener:      l,
		client:        apiClient,
		tunnels:       map[string]*wg.Tunnel{},
		currentChange: latestChangeAt,
		quit:          make(chan interface{}),
		background:    background,
	}

	err = s.serve(ctx, l)

	return
}

func bind() (net.Listener, error) {
	socket := agent.PathToSocket()

	if err := removeSocket(socket); err != nil {
		return nil, fmt.Errorf("failed removing existing socket: %w", err)
	}

	l, err := net.Listen("unix", socket)
	if err != nil {
		return nil, fmt.Errorf("failed binding: %w", err)
	}

	return l, nil
}

func latestChange() (at time.Time, err error) {
	var info os.FileInfo
	if info, err = os.Stat(flyctl.ConfigFilePath()); err != nil {
		err = fmt.Errorf("can't stat config file: %w", err)

		return
	}

	at = info.ModTime()

	return
}

var ErrTunnelUnavailable = errors.New("tunnel unavailable")

type server struct {
	logger        *log.Logger
	listener      net.Listener
	tunnels       map[string]*wg.Tunnel
	client        *api.Client
	lock          sync.Mutex
	currentChange time.Time
	quit          chan interface{}
	wg            sync.WaitGroup
	background    bool
}

type handlerFunc func(*server, context.Context, net.Conn, []string) error

var handlers = map[string]handlerFunc{
	"kill":      (*server).handleKill,
	"ping":      (*server).handlePing,
	"connect":   (*server).handleConnect,
	"probe":     (*server).handleProbe,
	"establish": (*server).handleEstablish,
	"instances": (*server).handleInstances,
	"resolve":   (*server).handleResolve,
}

var errShutdown = errors.New("shutdown")

func (s *server) serve(parent context.Context, l net.Listener) (err error) {
	eg, ctx := errgroup.WithContext(parent)

	eg.Go(func() error {
		<-ctx.Done()

		if err := l.Close(); err != nil {
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

		for {
			var conn net.Conn
			if conn, err = s.listener.Accept(); err == nil {
				eg.Go(func() error {
					defer func() {
						if err := conn.Close(); err != nil {
							s.printf("failed closing conn: %v", err)
						}
					}()

					s.handle(ctx, conn)
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

	if err = eg.Wait(); errors.Is(err, errShutdown) {
		err = nil
	}

	return
}

func (s *server) handle(ctx context.Context, conn net.Conn) {
	info, err := os.Stat(flyctl.ConfigFilePath())
	if err != nil {
		err = fmt.Errorf("can't stat config file: %w", err)

		s.error(conn, err)
		return
	}

	latestChange := info.ModTime()

	if latestChange.After(s.currentChange) {
		s.currentChange = latestChange
		if err := s.validateTunnels(); err != nil {
			s.error(conn, fmt.Errorf("can't validate peers: %w", err))
		}
		s.printf("config change at: %v", s.currentChange)
	}

	buf, err := proto.Read(conn)
	if err != nil {
		s.printf("failed reading command: %v", err)

		return
	}

	args := strings.Split(string(buf), " ")
	s.printf("received command: %v", args)

	if fn := handlers[args[0]]; fn == nil {
		s.error(conn, fmt.Errorf("unknown command: %v", args))
	} else {
		fn(s, ctx, conn, args)
	}
}

func (s *server) error(c net.Conn, err error) {
	_ = proto.Write(c, "err", err.Error())

	s.logger.Print(err)
}

func (s *server) handleKill(context.Context, net.Conn, []string) error {
	_ = s.listener.Close()

	return nil
}

func (s *server) handlePing(ctx context.Context, conn net.Conn, _ []string) (err error) {
	resp := agent.PingResponse{
		Version:    buildinfo.Version(),
		PID:        os.Getpid(),
		Background: s.background,
	}

	var data []byte
	if data, err = json.Marshal(resp); err != nil {
		err = fmt.Errorf("failed marshaling ping response: %w", err)
	} else {
		err = proto.Write(conn, "pong", string(data))
	}

	return
}

func findOrganization(ctx context.Context, client *api.Client, slug string) (*api.Organization, error) {
	orgs, err := client.GetOrganizations(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("can't load organizations from config: %w", err)
	}

	var org *api.Organization
	for _, o := range orgs {
		if o.Slug == slug {
			org = &o
			break
		}
	}

	if org == nil {
		return nil, errors.New("no such organization")
	}

	return org, nil
}

func buildTunnel(client *api.Client, org *api.Organization) (*wg.Tunnel, error) {
	state, err := wireguard.StateForOrg(client, org, "", "")
	if err != nil {
		return nil, fmt.Errorf("can't get wireguard state for %s: %w", org.Slug, err)
	}

	tunnel, err := wg.Connect(state)
	if err != nil {
		return nil, fmt.Errorf("can't connect wireguard: %w", err)
	}

	return tunnel, nil
}

func (s *server) handleEstablish(ctx context.Context, conn net.Conn, args []string) (err error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(args) != 2 {
		err = errors.New("malformed establish command")

		return
	}

	var org *api.Organization
	if org, err = findOrganization(ctx, s.client, args[1]); err != nil {
		return
	}

	tunnel, ok := s.tunnels[org.Slug]
	if !ok {
		tunnel, err = buildTunnel(s.client, org)
		if err != nil {
			return err
		}
		s.tunnels[org.Slug] = tunnel
	}

	resp := agent.EstablishResponse{
		WireGuardState: tunnel.State,
		TunnelConfig:   tunnel.Config,
	}

	var data []byte
	if data, err = json.Marshal(resp); err != nil {
		err = fmt.Errorf("failed marshaling establish response: %w", err)
	} else {
		err = proto.Write(conn, "ok", string(data))
	}

	return
}

func probeTunnel(ctx context.Context, tunnel *wg.Tunnel) (err error) {
	terminal.Debugf("Probing WireGuard connectivity\n")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	results, err := tunnel.LookupTXT(ctx, "_apps.internal")
	terminal.Debug("probe results for _apps.internal", results)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return ErrTunnelUnavailable
		}
		return errors.Wrap(err, "error probing for _apps.internal")
	}

	return nil
}

func (s *server) handleProbe(ctx context.Context, c net.Conn, args []string) error {
	tunnel, err := s.tunnelFor(args[1])
	if err != nil {
		return fmt.Errorf("probe: can't build tunnel: %s", err)
	}

	if err := probeTunnel(context.Background(), tunnel); err != nil {
		return err
	}

	proto.Write(c, "ok")

	return nil
}

func (s *server) handleResolve(ctx context.Context, conn net.Conn, args []string) error {
	if len(args) != 3 {
		return errors.New("malformed resolve command")
	}

	tunnel, err := s.tunnelFor(args[1])
	if err != nil {
		return fmt.Errorf("resolve: can't build tunnel: %s", err)
	}

	resp, err := resolve(tunnel, args[2])
	if err != nil {
		return err
	}

	proto.Write(conn, "ok", resp)

	return nil
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

func (s *server) handleInstances(ctx context.Context, c net.Conn, args []string) error {
	tunnel, err := s.tunnelFor(args[1])
	if err != nil {
		return fmt.Errorf("instance list: can't build tunnel: %w", err)
	}

	app := args[2]

	ret, err := s.fetchInstances(ctx, tunnel, app)
	if err != nil {
		return err
	}

	if len(ret.Addresses) == 0 {
		return fmt.Errorf("no running hosts for %s found", app)
	}

	var out bytes.Buffer
	if err := json.NewEncoder(&out).Encode(&ret); err != nil {
		panic(err)
	}

	return proto.Write(c, "ok", out.String())
}

func (s *server) handleConnect(ctx context.Context, conn net.Conn, args []string) error {
	s.printf("incoming connect: %v", args)

	if len(args) < 3 || len(args) > 4 {
		return fmt.Errorf("connect: malformed connect command: %v", args)
	}

	tunnel, err := s.tunnelFor(args[1])
	if err != nil {
		return fmt.Errorf("connect: can't build tunnel: %s", err)
	}

	address, err := resolve(tunnel, args[2])
	if err != nil {
		return fmt.Errorf("connect: can't resolve address '%s': %s", args[2], err)
	}

	var cancel func() = func() {}

	if len(args) > 3 {
		timeout, err := strconv.ParseUint(args[3], 10, 32)
		if err != nil {
			return fmt.Errorf("connect: invalid timeout: %s", err)
		}

		if timeout != 0 {
			ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
		}
	}

	outconn, err := tunnel.DialContext(ctx, "tcp", address)
	if err != nil {
		cancel()
		return fmt.Errorf("connection failed: %s", err)
	}

	cancel()

	defer outconn.Close()

	proto.Write(conn, "ok")

	var wg sync.WaitGroup
	wg.Add(2)

	copyFunc := func(dst net.Conn, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)

		// close the write half if it exports a CloseWrite() method
		if conn, ok := dst.(ClosableWrite); ok {
			conn.CloseWrite()
		}
	}

	go copyFunc(conn, outconn)
	go copyFunc(outconn, conn)
	wg.Wait()

	return nil
}

func (s *server) tunnelFor(slug string) (*wg.Tunnel, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	tunnel, ok := s.tunnels[slug]
	if !ok {
		return nil, fmt.Errorf("no tunnel for %s established", slug)
	}

	return tunnel, nil
}

// validateTunnels closes any active tunnel that isn't in the wire_guard_state config
func (s *server) validateTunnels() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	peers, err := wireguard.GetWireGuardState()
	if err != nil {
		return err
	}

	for slug, tunnel := range s.tunnels {
		if peers[slug] == nil {
			s.printf("no peer for %s in config - closing tunnel", slug)

			tunnel.Close()
			delete(s.tunnels, slug)
		}
	}

	return nil
}

func (s *server) clean(ctx context.Context) {
	for {
		if pause.For(ctx, 2*time.Minute); ctx.Err() != nil {
			break
		}

		if err := wireguard.PruneInvalidPeers(ctx, s.client); err != nil {
			s.printf("failed pruning invalid peers: %v", err)
		}

		if err := s.validateTunnels(); err != nil {
			s.printf("failed validating tunnels: %v", err)
		}

		s.printf("validated wireguard peers(stat)")
	}
}

func resolve(tunnel *wg.Tunnel, addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if strings.Contains(err.Error(), "missing port") {
			host = addr
		} else {
			return "", err
		}
	}

	if n := net.ParseIP(host); n != nil && n.To16() != nil {
		if port == "" {
			return n.String(), nil
		}
		return net.JoinHostPort(n.String(), port), nil
	}

	addrs, err := tunnel.LookupAAAA(context.Background(), host)
	if err != nil {
		return "", err
	}
	if len(addrs) == 0 {
		return "", fmt.Errorf("%s - no such host", addr)
	}

	if port == "" {
		return addrs[0].String(), nil
	}
	return net.JoinHostPort(addrs[0].String(), port), nil
}

func (s *server) print(v ...interface{}) {
	s.logger.Print(v...)
}

func (s *server) printf(format string, v ...interface{}) {
	s.logger.Printf(format, v...)
}

func removeSocket(path string) (err error) {
	var stat os.FileInfo
	switch stat, err = os.Stat(path); {
	case errors.Is(err, fs.ErrNotExist):
		err = nil
	case err != nil:
		break
	case stat.Mode()&os.ModeSocket == 0:
		err = errors.New("not a socket")
	default:
		err = os.Remove(path)
	}

	return
}

type ClosableWrite interface {
	CloseWrite() error
}
