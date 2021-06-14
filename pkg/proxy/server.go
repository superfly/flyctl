package proxy

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

type Server struct {
	LocalAddr string

	RemoteAddr string

	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
}

func (srv *Server) ServeTCP(ctx context.Context) error {
	addr, err := net.ResolveTCPAddr("tcp", srv.LocalAddr)
	if err != nil {
		return err
	}

	ls, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
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
				return err
			}
			defer source.Close()

			target, err := srv.Dial(ctx, "tcp", srv.RemoteAddr)
			if err != nil {
				return err
			}
			defer target.Close()

			go func() {
				wg := &sync.WaitGroup{}

				wg.Add(2)

				go func() {
					defer wg.Done()
					io.Copy(source, target)
				}()

				go func() {
					defer wg.Done()
					io.Copy(target, source)
				}()

				wg.Wait()
			}()
		}
	}
}
