package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/wireguard"
	"github.com/superfly/flyctl/pkg/wg"
	"github.com/superfly/flyctl/terminal"
)

var (
	ErrCantBind = errors.New("can't bind agent socket")
)

type Server struct {
	listener      *net.UnixListener
	ctx           context.Context
	cancel        func()
	tunnels       map[string]*wg.Tunnel
	client        *api.Client
	lock          sync.Mutex
	currentChange time.Time
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

	buf, err := read(c)
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

func NewServer(path string, apiClient *api.Client) (*Server, error) {
	if c, err := NewClient(path, apiClient); err == nil {
		c.Kill(context.Background())
	}

	if err := removeSocket(path); err != nil {
		// most of these errors just mean the socket isn't already there
		// which is what we want.

		if errors.Is(err, ErrCantBind) {
			return nil, err
		}
	}

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		fmt.Printf("Failed to resolve: %v\n", err)
		os.Exit(1)
	}

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("can't bind: %w", err)
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
	}

	s.ctx, s.cancel = context.WithCancel(context.Background())

	return s, nil
}

func DefaultServer(apiClient *api.Client) (*Server, error) {
	wireguard.PruneInvalidPeers(apiClient)

	return NewServer(fmt.Sprintf("%s/.fly/fly-agent.sock", os.Getenv("HOME")), apiClient)
}

func (s *Server) Serve() {
	defer s.listener.Close()
	defer s.cancel()

	go s.clean()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			// this can't really be how i'm supposed to do this
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}

			log.Printf("warning: couldn't accept connection: %s", err)
			continue
		}

		go s.handle(conn)
	}
}

func (s *Server) errLog(c net.Conn, format string, args ...interface{}) {
	writef(c, "err "+format, args...)
	log.Printf(format, args...)
}

func (s *Server) handleKill(c net.Conn, args []string) error {
	s.listener.Close()
	return nil
}

func (s *Server) handlePing(c net.Conn, args []string) error {
	return writef(c, "pong %d", os.Getpid())
}

func findOrganization(client *api.Client, slug string) (*api.Organization, error) {
	orgs, err := client.GetOrganizations(nil)
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

	tunnel, err := wg.Connect(*state.TunnelConfig())
	if err != nil {
		// captureWireguardConnErr(err, org.Slug)
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

	if _, ok := s.tunnels[org.Slug]; ok {
		return writef(c, "ok")
	}

	tunnel, err := buildTunnel(s.client, org)
	if err != nil {
		return err
	}

	s.tunnels[org.Slug] = tunnel
	return writef(c, "ok")
}

func probeTunnel(ctx context.Context, tunnel *wg.Tunnel) error {
	var err error

	terminal.Debugf("Probing WireGuard connectivity\n")

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err = tunnel.Resolver().LookupTXT(ctx, "_apps.internal")
	if err != nil {
		return fmt.Errorf("probing look up apps: %w", err)
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
		// captureWireguardConnErr(err, args[1])
		return err
	}

	writef(c, "ok")

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

	writef(c, "ok "+resp)

	return nil
}

type Instances struct {
	Labels    []string
	Addresses []string
}

func fetchInstances(tunnel *wg.Tunnel, app string) (*Instances, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	regionsv, err := tunnel.Resolver().
		LookupTXT(ctx, fmt.Sprintf("regions.%s.internal", app))
	if err != nil {
		return nil, fmt.Errorf("look up regions for %s: %w", app, err)
	}

	regions := strings.Trim(regionsv[0], " \t")
	if regions == "" {
		return nil, fmt.Errorf("can't find deployed regions for %s", app)
	}

	ret := &Instances{}

	for _, region := range strings.Split(regions, ",") {
		name := fmt.Sprintf("%s.%s.internal", region, app)
		addrs, err := tunnel.Resolver().LookupHost(ctx, name)
		if err != nil {
			log.Printf("can't lookup records for %s: %s", name, err)
			continue
		}

		if len(addrs) == 1 {
			ret.Labels = append(ret.Labels, name)
			ret.Addresses = append(ret.Addresses, addrs[0])
			continue
		}

		for _, addr := range addrs {
			ret.Labels = append(ret.Labels, fmt.Sprintf("%s (%s)", region, addr))
			ret.Addresses = append(ret.Addresses, addrs[0])
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

	return writef(c, "ok %s", out.String())
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
		// captureWireguardConnErr(err, args[1])
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
		// captureWireguardConnErr(err, args[1])
		cancel()
		return fmt.Errorf("connection failed: %s", err)
	}

	cancel()

	defer outconn.Close()

	writef(c, "ok")

	wg := &sync.WaitGroup{}
	wg.Add(2)

	copy := func(dst net.Conn, src net.Conn) {
		defer wg.Done()
		io.Copy(dst, src)

		// close the write half if it exports a CloseWrite() method
		if conn, ok := dst.(ClosableWrite); ok {
			conn.CloseWrite()
		}
	}

	go copy(c, outconn)
	go copy(outconn, c)
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
			if err := wireguard.PruneInvalidPeers(s.client); err != nil {
				log.Printf("failed to prune invalid peers: %s", err)
			}
			if err := s.validateTunnels(); err != nil {
				log.Printf("failed to validate tunnels: %s", err)
			}
			log.Printf("validated wireguard peers(stat)")
		case <-s.ctx.Done():
			return
		}
	}
}

func resolve(tunnel *wg.Tunnel, addr string) (string, error) {
	log.Printf("Resolving %v %s", tunnel, addr)
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

	addrs, err := tunnel.Resolver().LookupHost(context.Background(), host)
	if err != nil {
		return "", err
	}

	if port == "" {
		return addrs[0], nil
	}
	return net.JoinHostPort(addrs[0], port), nil
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
