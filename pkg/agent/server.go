package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func DefaultServer(apiClient *api.Client, background bool) (*Server, error) {
	return NewServer(pathToSocket(), apiClient, background)
}

func NewServer(path string, apiClient *api.Client, background bool) (*Server, error) {
	if err := removeSocket(path); err != nil {
		// most of these errors just mean the socket isn't already there
		// which is what we want.

		if errors.Is(err, ErrCantBind) {
			return nil, err
		}
	}

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, errors.Wrap(err, "can't resolve unix socket")
	}

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, errors.Wrap(err, "can't bind")
	}

	l.SetUnlinkOnClose(true)

	info, err := os.Stat(flyctl.ConfigFilePath())
	if err != nil {
		return nil, fmt.Errorf("can't stat config file: %s", err)
	}

	latestChange := info.ModTime()

	s := &Server{
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
	ErrCantBind          = errors.New("can't bind agent socket")
	ErrTunnelUnavailable = errors.New("tunnel unavailable")
)

type Server struct {
	listener      *net.UnixListener
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
		s.errLog(c, "can't stat config file: %s", err)
		return
	}

	latestChange := info.ModTime()

	if latestChange.After(s.currentChange) {
		s.currentChange = latestChange
		err := s.validateTunnels()
		if err != nil {
			s.errLog(c, "can't validate peers: %s", err)
		}
		log.Printf("config change at: %s", s.currentChange.String())
	}

	buf, err := proto.Read(c)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			log.Printf("couldn't read command: %s", err)
		}
		return
	}

	args := strings.Split(string(buf), " ")

	log.Printf("incoming command: %v", args)

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
		s.errLog(c, "bad command: %v", args)
		return
	}

	if err = handler(c, args); err != nil {
		s.errLog(c, "err handling %s: %s", args[0], err)
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
					log.Printf("warning: couldn't accept connection: %s", err)
					continue
				}
			}

			go s.handle(conn)
		}
	}()
}

func (*Server) errLog(c net.Conn, format string, args ...interface{}) {
	proto.Writef(c, "err "+format, args...)
	log.Printf(format, args...)
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

	return proto.Writef(c, "pong %s", data)
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
	return proto.Writef(c, "ok %s", data)
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

func fetchInstances(tunnel *wg.Tunnel, app string) (*Instances, error) {
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
			log.Printf("can't lookup records for %s: %s", name, err)
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

	ret, err := fetchInstances(tunnel, app)
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
	log.Printf("incoming connect: %v", args)

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
			log.Printf("no peer for %s in config - closing tunnel", slug)
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
				log.Printf("failed to prune invalid peers: %s", err)
			}
			if err := s.validateTunnels(); err != nil {
				log.Printf("failed to validate tunnels: %s", err)
			}
			log.Printf("validated wireguard peers(stat)")
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

func removeSocket(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}

	if stat.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%w: refusing to remove something that isn't a socket", ErrCantBind)
	}

	return os.Remove(path)
}

type ClosableWrite interface {
	CloseWrite() error
}
