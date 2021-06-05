package proxy

import (
	"context"
	"io"
	"net"
	"sync"

	"github.com/superfly/flyctl/terminal"
)

type Server struct {
	LocalAddr string

	RemoteAddr string

	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (srv *Server) ListenAndServe(ctx context.Context) error {

	ls, err := net.Listen("tcp", srv.LocalAddr)
	if err != nil {
		return err
	}
	defer ls.Close()

	for {
		lConn, err := ls.Accept()
		if err != nil {
			return err
		}

		wg := &sync.WaitGroup{}

		wg.Add(1)

		go func(conn net.Conn) {
			err := srv.proxy(ctx, lConn)
			terminal.Debug(err)
			wg.Done()
		}(lConn)
	}
}

func (srv *Server) proxy(ctx context.Context, lConn net.Conn) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	rConn, err := srv.Dial(ctx, "tcp", srv.RemoteAddr)
	if err != nil {
		return err
	}
	defer rConn.Close()

	errChan := make(chan error, 1)

	go func() {
		_, err := io.Copy(lConn, rConn)
		errChan <- err
	}()

	go func() {
		_, err := io.Copy(rConn, lConn)
		errChan <- err
	}()

	select {
	case <-ctx.Done():
		break
	case err := <-errChan:
		if err == io.EOF {
			break
		}
		return err
	}

	return nil
}
