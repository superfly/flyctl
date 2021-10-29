package proxy

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/superfly/flyctl/terminal"
)

type Server struct {
	LocalAddr string
	Addr      string
	Listener  net.Listener
	Dial      func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (srv *Server) Proxy(ctx context.Context) error {

	ls, ok := srv.Listener.(*net.TCPListener)
	if !ok {
		return nil
	}

	defer ls.Close()

	for {
		select {

		case <-ctx.Done():
			return nil
		default:
			if err := ls.SetDeadline(time.Now().Add(time.Second)); err != nil {
				return err
			}

			source, err := ls.Accept()
			if err != nil {
				if os.IsTimeout(err) {
					continue
				}
				terminal.Debug("Error accepting connection: ", err)
			}
			defer source.Close()

			terminal.Debug("accepted new connection from: ", source.RemoteAddr())

			go func() {
				target, err := srv.Dial(ctx, "tcp", srv.Addr)
				if err != nil {
					terminal.Debug("failed to connect to target: ", err)
					return
				}
				defer target.Close()

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

				go copyFunc(target, source)
				go copyFunc(source, target)

				wg.Wait()

				terminal.Debug("connection closed")
			}()
		}
	}
}

type ClosableWrite interface {
	CloseWrite() error
}
