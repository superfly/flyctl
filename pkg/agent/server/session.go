package server

import (
	"bytes"
	"context"
	"encoding/binary"
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

	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/api"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/agent/internal/proto"
	"github.com/superfly/flyctl/pkg/wg"

	"github.com/superfly/flyctl/internal/buildinfo"
)

type id uint64

func (id id) String() string {
	return fmt.Sprintf("#%x", uint64(id))
}

type session struct {
	srv    *server
	conn   net.Conn
	logger *log.Logger
	id     id
}

var errUnsupportedCommand = errors.New("unsupported command")

func runSession(ctx context.Context, srv *server, conn net.Conn, id id) {
	var wg sync.WaitGroup
	defer wg.Wait()

	wg.Add(1)
	go func() {
		defer wg.Done()

		<-ctx.Done()
		_ = conn.Close()
	}()

	logger := log.New(srv.Logger.Writer(), id.String()+" ", srv.Logger.Flags())
	logger.Print("connected ...")

	defer func() {
		defer func() {
			if err := conn.Close(); err != nil && !isClosed(err) {
				logger.Printf("failed dropping: %v", err)
			} else {
				logger.Print("dropped.")
			}
		}()
	}()

	s := &session{
		srv:    srv,
		conn:   conn,
		logger: logger,
		id:     id,
	}

	if err := s.srv.checkForConfigChange(); err != nil {
		s.error(err)

		return
	}

	buf, err := proto.Read(s.conn)
	if len(buf) > 0 {
		s.logger.Printf("<- (% 5d) %q", len(buf), buf)
	}

	if err != nil {
		if !isClosed(err) {
			s.logger.Printf("failed reading: %v", err)
		}

		return
	}

	args := strings.Split(string(buf), " ")

	fn := handlers[args[0]]
	if fn == nil {
		s.error(errUnsupportedCommand)

		return
	}

	fn(s, ctx, args[1:]...)
}

type handlerFunc func(*session, context.Context, ...string)

var handlers = map[string]handlerFunc{
	"kill":        (*session).kill,
	"ping":        (*session).ping,
	"establish":   (*session).establish,
	"reestablish": (*session).reestablish,
	"connect":     (*session).connect,
	"probe":       (*session).probe,
	"instances":   (*session).instances,
	"resolve":     (*session).resolve,
	"ping6":       (*session).ping6,
}

var errMalformedKill = errors.New("malformed kill command")

func (s *session) kill(_ context.Context, args ...string) {
	if !s.noArgs(args, errMalformedKill) {
		return
	}

	s.ok()

	s.srv.shutdown()
}

var errMalformedPing = errors.New("malformed ping command")

func (s *session) ping(_ context.Context, args ...string) {
	if !s.noArgs(args, errMalformedPing) {
		return
	}

	_ = s.marshal(agent.PingResponse{
		Version:    buildinfo.Version(),
		PID:        os.Getpid(),
		Background: s.srv.Options.Background,
	})
}

var (
	errMalformedEstablish = errors.New("malformed establish command")
)

func (s *session) doEstablish(ctx context.Context, recycle bool, args ...string) {
	if !s.exactArgs(1, args, errMalformedEstablish) {
		return
	}

	org, err := s.fetchOrg(ctx, args[0])
	if err != nil {
		s.error(err)

		return
	}

	tunnel, err := s.srv.buildTunnel(org, recycle)
	if err != nil {
		s.error(err)

		return
	}

	_ = s.marshal(agent.EstablishResponse{
		WireGuardState: tunnel.State,
		TunnelConfig:   tunnel.Config,
	})
}

func (s *session) establish(ctx context.Context, args ...string) {
	s.doEstablish(ctx, false, args...)
}

func (s *session) reestablish(ctx context.Context, args ...string) {
	s.doEstablish(ctx, true, args...)
}

var errNoSuchOrg = errors.New("no such organization")

func (s *session) fetchOrg(ctx context.Context, slug string) (*api.Organization, error) {
	orgs, err := s.srv.Client.GetOrganizations(ctx, nil)
	if err != nil {
		return nil, err
	}

	for _, org := range orgs {
		if org.Slug == slug {
			no := org // copy
			return &no, nil
		}
	}

	return nil, errNoSuchOrg
}

var errMalformedProbe = errors.New("malformed probe command")

func (s *session) probe(ctx context.Context, args ...string) {
	if !s.exactArgs(1, args, errMalformedProbe) {
		return
	}

	if err := s.srv.probeTunnel(ctx, args[0]); err != nil {
		s.error(err)

		return
	}

	_ = s.ok()
}

var errMalformedInstances = errors.New("malformed instances command")

func (s *session) instances(ctx context.Context, args ...string) {
	if !s.exactArgs(2, args, errMalformedInstances) {
		return
	}

	tunnel := s.srv.tunnelFor(args[0])
	if tunnel == nil {
		s.error(agent.ErrTunnelUnavailable)

		return
	}

	app := args[1]

	ret, err := s.srv.fetchInstances(ctx, tunnel, app)
	if err != nil {
		err = fmt.Errorf("failed fetching instances for %q: %w", app, err)
		s.error(err)

		return
	}

	if len(ret.Addresses) == 0 {
		err = fmt.Errorf("no running hosts for %q found", app)
		s.error(err)

		return
	}

	_ = s.marshal(ret)
}

var errMalformedResolve = errors.New("malformed resolve command")

func (s *session) resolve(ctx context.Context, args ...string) {
	if !s.exactArgs(2, args, errMalformedResolve) {
		return
	}

	tunnel := s.srv.tunnelFor(args[0])
	if tunnel == nil {
		s.error(agent.ErrTunnelUnavailable)

		return
	}

	addr, err := resolve(ctx, tunnel, args[1])
	if err != nil {
		s.error(err)

		return
	}

	s.ok(addr)
}

func resolve(ctx context.Context, tunnel *wg.Tunnel, addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		if !strings.Contains(err.Error(), "missing port") {
			return "", err
		}

		host = addr
	}

	if n := net.ParseIP(host); n != nil && n.To16() != nil {
		if port == "" {
			return n.String(), nil
		}

		return net.JoinHostPort(n.String(), port), nil
	}

	ips, err := tunnel.LookupAAAA(ctx, host)
	if err != nil {
		return "", err
	}

	if len(ips) == 0 {
		return "", agent.ErrNoSuchHost
	}

	addr = ips[0].String()
	if port != "" {
		addr = net.JoinHostPort(addr, port)
	}

	return addr, nil
}

var (
	errMalformedConnect = errors.New("malformed connect command")
	errInvalidTimeout   = errors.New("invalid timeout")
	errDone             = errors.New("done")
)

func (s *session) connect(ctx context.Context, args ...string) {
	if !s.exactArgs(3, args, errMalformedConnect) {
		return
	}

	timeout, err := strconv.ParseUint(args[2], 10, 32)
	if err != nil {
		s.error(err)

		return
	}

	tunnel := s.srv.tunnelFor(args[0])
	if tunnel == nil {
		s.error(agent.ErrTunnelUnavailable)

		return
	}

	var dialContext context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		dialContext, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	} else {
		dialContext, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	outconn, err := tunnel.DialContext(dialContext, "tcp", args[1])
	if err != nil {
		s.error(err)

		return
	}
	defer func() {
		if err := outconn.Close(); err != nil && !isClosed(err) {
			s.logger.Printf("failed closing outconn: %v", err)
		}
	}()

	if !s.ok() {
		return
	}

	var eg *errgroup.Group
	eg, ctx = errgroup.WithContext(ctx)

	eg.Go(func() error {
		<-ctx.Done()
		_ = s.conn.Close()
		_ = outconn.Close()

		return errDone
	})

	eg.Go(func() (err error) {
		if _, err = io.Copy(s.conn, outconn); err == nil {
			err = io.EOF
		}

		return
	})

	eg.Go(func() (err error) {
		if _, err = io.Copy(outconn, s.conn); err == nil {
			err = io.EOF
		}

		return
	})

	_ = eg.Wait()
}

func (s *session) ping6(ctx context.Context, args ...string) {

	// As with "dial", "ping6" handles an agent command and then
	// repurposes the agent connection as a transport.

	if len(args) != 1 {
		s.error(fmt.Errorf("ping6: bad args"))
		return
	}

	tunnel := s.srv.tunnelFor(args[0])
	if tunnel == nil {
		s.error(agent.ErrTunnelUnavailable)
		return
	}

	// YOG-SOTHOTH IS THE GATE
	sock, err := tunnel.ListenPing()
	if err != nil {
		s.error(fmt.Errorf("ping6: %w", err))
		return
	}

	ctx, cancel := context.WithCancel(ctx)

	// a background thread watches for incoming ICMP messages on
	// the ICMP "socket" we get from wireguard-go. Each received
	// message is relayed back over the agent connection.

	go func() {
		pbuf := make([]byte, 1500)
		lbuf := make([]byte, 2)

		for {
			if ctx.Err() != nil {
				return
			}

			sock.SetDeadline(time.Now().Add(1 * time.Second))
			n64, addr, err := sock.ReadFrom(pbuf)
			if err != nil {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					s.logger.Printf("ping6: %s", err)
				}

				continue
			}

			v6addr := net.ParseIP(addr.String()).To16()
			if v6addr == nil {
				s.logger.Printf("ping6: bad remote address '%s'", addr)
				continue
			}

			payload := pbuf[:n64]
			binary.BigEndian.PutUint16(lbuf, uint16(len(payload)))

			// not much point to handling the error here; the parent
			// thread will cancel this goroutine if the connection dies.
			s.conn.Write(v6addr)
			s.conn.Write(lbuf)
			s.conn.Write(payload)
		}
	}()

	pbuf := make([]byte, 1500)
	lbuf := make([]byte, 2)
	v6buf := make([]byte, 16)

	sockOk := func(err error) bool {
		if err == nil {
			return true
		}

		if err != io.EOF {
			s.logger.Printf("ping6: socket read error: %s", err)
		}

		cancel()

		return false
	}

	for {
		_, err = io.ReadFull(s.conn, v6buf)
		if !sockOk(err) {
			return
		}

		_, err = io.ReadFull(s.conn, lbuf)
		if !sockOk(err) {
			return
		}

		paylen := binary.BigEndian.Uint16(lbuf)
		if paylen >= 1500 {
			sockOk(fmt.Errorf("bad payload length (>=1500)"))
			return
		}

		pkt := pbuf[:paylen]
		_, err = io.ReadFull(s.conn, pkt)
		if !sockOk(err) {
			return
		}

		_, err = sock.WriteTo(pkt, &net.IPAddr{IP: net.IP(v6buf)})
		if !sockOk(err) {
			return
		}
	}
}

func (s *session) error(err error) bool {
	return s.reply("err", err.Error())
}

func (s *session) ok(args ...string) bool {
	return s.reply("ok", args...)
}

func (s *session) reply(verb string, args ...string) bool {
	var b bytes.Buffer
	out := io.MultiWriter(
		&b,
		s.conn,
	)

	err := proto.Write(out, verb, args...)
	if l := b.Len(); l > 0 {
		s.logger.Printf("-> (% 5d) %q", l, b.Bytes())
	}

	if err != nil {
		if !isClosed(err) {
			s.logger.Printf("failed writing: %v", err)
		}

		return false
	}

	return true
}

func (s *session) noArgs(args []string, err error) bool {
	if len(args) != 0 {
		s.error(err)

		return false
	}

	return true
}

func (s *session) exactArgs(count int, args []string, err error) bool {
	if len(args) != count {
		s.error(err)

		return false
	}

	return true
}

func (s *session) minMaxArgs(min, max int, args []string, err error) bool {
	if len(args) < min || len(args) > max {
		s.error(err)

		return false
	}

	return true
}

func (s *session) marshal(v interface{}) (ok bool) {
	var sb strings.Builder

	enc := json.NewEncoder(&sb)
	switch err := enc.Encode(v); err {
	default:
		err = fmt.Errorf("failed marshaling response: %w", err)

		s.error(err)
	case nil:
		ok = true

		s.ok(sb.String())
	}

	return
}

func isClosed(err error) bool {
	return errors.Is(err, net.ErrClosed)
}
