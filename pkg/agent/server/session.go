package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/superfly/flyctl/internal/buildinfo"
	"github.com/superfly/flyctl/pkg/agent"
	"github.com/superfly/flyctl/pkg/agent/internal/proto"
	"golang.org/x/sync/errgroup"
)

type session struct {
	srv  *server
	conn net.Conn
}

func (s *session) run(ctx context.Context) {
	if err := s.srv.checkForConfigChange(); err != nil {
		s.error(err)

		return
	}

	buf, err := proto.Read(s.conn)
	if err != nil {
		s.srv.printf("failed reading command: %v", err)

		return
	}

	args := strings.Split(string(buf), " ")
	s.srv.printf("received command: %v", args)

	fn := handlers[args[0]]
	if fn == nil {
		s.error(fmt.Errorf("unknown command: %v", args))

		return
	}

	fn(s, ctx, args[1:]...)
}

type handlerFunc func(*session, context.Context, ...string)

var handlers = map[string]handlerFunc{
	"kill":      (*session).kill,
	"ping":      (*session).ping,
	"establish": (*session).establish,
	"connect":   (*session).connect,
	"probe":     (*session).probe,
	"instances": (*session).instances,
	"resolve":   (*session).resolve,
}

var errMalformedKill = errors.New("malformed kill command")

func (s *session) kill(_ context.Context, args ...string) {
	if !s.noArgs(args, errMalformedKill) {
		return
	}

	s.srv.shutdown()

	s.ok()
}

var errMalformedPing = errors.New("malformed ping command")

func (s *session) ping(_ context.Context, args ...string) {
	if !s.noArgs(args, errMalformedPing) {
		return
	}

	resp := agent.PingResponse{
		Version:    buildinfo.Version(),
		PID:        os.Getpid(),
		Background: s.srv.Options.Background,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		err = fmt.Errorf("failed marshaling ping response: %w", err)
		s.error(err)

		return
	}

	s.reply("pong", string(data))
}

var errMalformedEstablish = errors.New("malformed establish command")

func (s *session) establish(ctx context.Context, args ...string) {
	if !s.exactArgs(1, args, errMalformedEstablish) {
		return
	}

	org, err := s.srv.findOrganization(ctx, args[0])
	if err != nil {
		s.error(err)

		return
	}

	tunnel, err := s.srv.buildTunnel(org)
	if err != nil {
		err = fmt.Errorf("failed building tunnel: %w", err)
		s.error(err)

		return
	}

	res := agent.EstablishResponse{
		WireGuardState: tunnel.State,
		TunnelConfig:   tunnel.Config,
	}

	data, err := json.Marshal(res)
	if err != nil {
		err = fmt.Errorf("failed marshaling establish response: %w", err)
		s.error(err)

		return
	}

	s.reply("ok", string(data))
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

	tunnel, err := s.srv.tunnelFor(args[0])
	if err != nil {
		s.error(err)

		return
	}

	app := args[2]

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

	data, err := json.Marshal(ret)
	if err != nil {
		err = fmt.Errorf("failed marshaling instances response: %w", err)
		s.error(err)

		return
	}

	s.reply("ok", string(data))
}

var errMalformedResolve = errors.New("malformed resolve command")

func (s *session) resolve(ctx context.Context, args ...string) {
	if !s.exactArgs(2, args, errMalformedResolve) {
		return
	}

	tunnel, err := s.srv.tunnelFor(args[0])
	if err != nil {
		s.error(err)

		return
	}

	resp, err := resolve(ctx, tunnel, args[1])
	if err != nil {
		s.error(err)

		return
	}

	s.reply("ok", resp)
}

var (
	errMalformedConnect = errors.New("malformed connect command")
	errDone             = errors.New("done")
)

func (s *session) connect(ctx context.Context, args ...string) {
	if !s.exactArgs(3, args, errMalformedConnect) {
		return
	}
	s.srv.printf("incoming connect: %v", args)

	tunnel, err := s.srv.tunnelFor(args[1])
	if err != nil {
		err = fmt.Errorf("connect: can't build tunnel: %w", err)
		s.error(err)

		return
	}

	address, err := resolve(ctx, tunnel, args[2])
	if err != nil {
		err = fmt.Errorf("connect: can't resolve address %q: %w", args[2], err)
		s.error(err)

		return
	}

	var cancel context.CancelFunc = func() {}

	if len(args) > 3 {
		timeout, err := strconv.ParseUint(args[3], 10, 32)
		if err != nil {
			err = fmt.Errorf("connect: invalid timeout: %s", err)
			s.error(err)

			return
		}

		if timeout != 0 {
			ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
		}
	}
	defer cancel()

	outconn, err := tunnel.DialContext(ctx, "tcp", address)
	if err != nil {
		err = fmt.Errorf("connection failed: %w", err)
		s.error(err)

		return
	}
	defer func() {
		if err := outconn.Close(); err != nil {
			s.srv.printf("failed closing outconn: %v", err)
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

func (s *session) error(err error) bool {
	s.srv.print(err)

	return s.reply("err", err.Error())
}

func (s *session) ok() bool {
	return s.reply("ok")
}

func (s *session) reply(verb string, args ...string) bool {
	if err := proto.Write(s.conn, verb, args...); err != nil {
		s.srv.printf("failed writing: %v", err)

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
