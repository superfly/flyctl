package agent

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

	"github.com/pkg/errors"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/pkg/agent/internal/proto"
	"github.com/superfly/flyctl/pkg/wg"
	"github.com/superfly/flyctl/terminal"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
)

func DefaultServer(logger *log.Logger, apiClient *api.Client, background bool) (*Server, error) {
	return newServer(logger, pathToSocket(), apiClient, background)
}

func newServer(logger *log.Logger, path string, apiClient *api.Client, background bool) (*Server, error) {
	if err := removeSocket(path); err != nil {
		return nil, fmt.Errorf("failed removing existing socket: %w", err)
	}

	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("failed binding on %s: %w", path, err)
	}

	info, err := os.Stat(flyctl.ConfigFilePath())
	if err != nil {
		return nil, fmt.Errorf("can't stat config file: %w", err)
	}

	latestChange := info.ModTime()

	s := &Server{
		logger:        logger,
		listener:      l,
		client:        apiClient,
		tunnels:       map[string]*wg.Tunnel{},
		currentChange: latestChange,
		quit:          make(chan interface{}),
		background:    background,
	}

	return s, nil
}

var (
	ErrTunnelUnavailable = errors.New("tunnel unavailable")
)

type Server struct {
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

type handlerFunc func(net.Conn, []string) error

func (s *Server) handle(c net.Conn) {
	defer c.Close()

	info, err := os.Stat(flyctl.ConfigFilePath())
	if err != nil {
		s.errorf(c, "can't stat config file: %s", err)
		return
	}

	latestChange := info.ModTime()

	if latestChange.After(s.currentChange) {
		s.currentChange = latestChange
		err := s.validateTunnels()
		if err != nil {
			s.errorf(c, "can't validate peers: %s", err)
		}
		s.logger.Printf("config change at: %s", s.currentChange.String())
	}

	buf, err := proto.Read(c)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			s.logger.Printf("couldn't read command: %s", err)
		}
		return
	}

	args := strings.Split(string(buf), " ")

	s.logger.Printf("incoming command: %v", args)

	cmds := map[string]handlerFunc{
		"kill":      s.handleKill,
		"ping":      s.handlePing,
		"connect":   s.handleConnect,
		"probe":     s.handleProbe,
		"establish": s.handleEstablish,
		"instances": s.handleInstances,
		"resolve":   s.handleResolve,
	}

	handler, ok := cmds[args[0]]
	if !ok {
		s.errorf(c, "bad command: %v", args)

		return
	}

	if err = handler(c, args); err != nil {
		s.errorf(c, "err handling %s: %s", args[0], err)

		return
	}
}

func (s *Server) Stop() {
	s.listener.Close()
	close(s.quit)
}

func (s *Server) Wait() {
	s.wg.Wait()
}

func (s *Server) Serve() {
	go s.clean()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		for {
			conn, err := s.listener.Accept()

			if err != nil {
				select {
				case <-s.quit:
					return
				default:
					if errors.Is(err, net.ErrClosed) {
						return
					}
					s.logger.Printf("warning: couldn't accept connection: %s", err)
					continue
				}
			}

			go s.handle(conn)
		}
	}()
}

func (s *Server) errorf(c net.Conn, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	proto.Write(c, "err", msg)

	s.logger.Print(msg)
}

func (s *Server) handleKill(_ net.Conn, _ []string) error {
	s.Stop()

	return nil
}

func (s *Server) handlePing(c net.Conn, _ []string) error {
	resp := PingResponse{
		Version:    buildinfo.Version(),
		PID:        os.Getpid(),
		Background: s.background,
	}

	data, _ := json.Marshal(resp)

	return proto.Write(c, "pong", string(data))
}

func findOrganization(client *api.Client, slug string) (*api.Organization, error) {
	orgs, err := client.GetOrganizations(context.TODO(), nil)
	if err != nil {
		return nil, fmt.Errorf("can't load organizations from config: %s", err)
	}

	var org *api.Organization
	for _, o := range orgs {
		if o.Slug == slug {
			org = &o
			break
		}
	}

	if org == nil {
		return nil, fmt.Errorf("no such organization")
	}

	return org, nil
}

func buildTunnel(client *api.Client, org *api.Organization) (*wg.Tunnel, error) {
	state, err := wireguard.StateForOrg(client, org, "", "")
	if err != nil {
		return nil, fmt.Errorf("can't get wireguard state for %s: %s", org.Slug, err)
	}

	tunnel, err := wg.Connect(state)
	if err != nil {
		return nil, fmt.Errorf("can't connect wireguard: %w", err)
	}

	return tunnel, nil
}

// handleEstablish establishes a new wireguard tunnel to an organization.
func (s *Server) handleEstablish(c net.Conn, args []string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(args) != 2 {
		return fmt.Errorf("malformed establish command")
	}

	org, err := findOrganization(s.client, args[1])
	if err != nil {
		return err
	}

	tunnel, ok := s.tunnels[org.Slug]
	if !ok {
		tunnel, err = buildTunnel(s.client, org)
		if err != nil {
			return err
		}
		s.tunnels[org.Slug] = tunnel
	}

	resp := EstablishResponse{
		WireGuardState: tunnel.State,
		TunnelConfig:   tunnel.Config,
	}

	data, _ := json.Marshal(resp)
	return proto.Write(c, "ok", string(data))
}

func probeTunnel(ctx context.Context, tunnel *wg.Tunnel) error {
	var err error

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

// handleProbe probes a wireguard tunnel to see if it's still alive.
func (s *Server) handleProbe(c net.Conn, args []string) error {
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

// handleResolve resolves the provided host with the tunnel
func (s *Server) handleResolve(c net.Conn, args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("malformed resolve command")
	}

	tunnel, err := s.tunnelFor(args[1])
	if err != nil {
		return fmt.Errorf("resolve: can't build tunnel: %s", err)
	}

	resp, err := resolve(tunnel, args[2])
	if err != nil {
		return err
	}

	proto.Write(c, "ok", resp)

	return nil
}

func (s *Server) fetchInstances(tunnel *wg.Tunnel, app string) (*Instances, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	ret := &Instances{}

	for _, region := range strings.Split(regions, ",") {
		name := fmt.Sprintf("%s.%s.internal", region, app)
		addrs, err := tunnel.LookupAAAA(ctx, name)
		if err != nil {
			s.logger.Printf("can't lookup records for %s: %s", name, err)
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

// handleInstances returns a list of instances of an app.
func (s *Server) handleInstances(c net.Conn, args []string) error {
	tunnel, err := s.tunnelFor(args[1])
	if err != nil {
		return fmt.Errorf("instance list: can't build tunnel: %s", err)
	}

	app := args[2]

	ret, err := s.fetchInstances(tunnel, app)
	if err != nil {
		return err
	}

	if len(ret.Addresses) == 0 {
		return fmt.Errorf("no running hosts for %s found", app)
	}

	out := &bytes.Buffer{}
	json.NewEncoder(out).Encode(&ret)

	return proto.Write(c, "ok", out.String())
}

func (s *Server) handleConnect(c net.Conn, args []string) error {
	s.logger.Printf("incoming connect: %v", args)

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

	ctx := context.Background()
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

	proto.Write(c, "ok")

	wg := &sync.WaitGroup{}
	wg.Add(2)

	copyFunc := func(dst net.Conn, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)

		// close the write half if it exports a CloseWrite() method
		if conn, ok := dst.(ClosableWrite); ok {
			conn.CloseWrite()
		}
	}

	go copyFunc(c, outconn)
	go copyFunc(outconn, c)
	wg.Wait()

	return nil
}

func (s *Server) tunnelFor(slug string) (*wg.Tunnel, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	tunnel, ok := s.tunnels[slug]
	if !ok {
		return nil, fmt.Errorf("no tunnel for %s established", slug)
	}

	return tunnel, nil
}

// validateTunnels closes any active tunnel that isn't in the wire_guard_state config
func (s *Server) validateTunnels() error {
	s.lock.Lock()
	defer s.lock.Unlock()

	peers, err := wireguard.GetWireGuardState()
	if err != nil {
		return err
	}

	for slug, tunnel := range s.tunnels {
		if peers[slug] == nil {
			s.logger.Printf("no peer for %s in config - closing tunnel", slug)
			tunnel.Close()
			delete(s.tunnels, slug)
		}
	}

	return nil
}

func (s *Server) clean() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := wireguard.PruneInvalidPeers(context.TODO(), s.client); err != nil {
				s.logger.Printf("failed to prune invalid peers: %s", err)
			}
			if err := s.validateTunnels(); err != nil {
				s.logger.Printf("failed to validate tunnels: %s", err)
			}
			s.logger.Printf("validated wireguard peers(stat)")
		case <-s.quit:
			return
		}
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
