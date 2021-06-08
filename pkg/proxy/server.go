package proxy

import (
	"context"
	"io"
	"net"
	"sync"
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
		in, err := ls.Accept()
		if err != nil {
			return err
		}
		defer in.Close()

		go func() error {

			out, err := srv.Dial(ctx, "tcp", srv.RemoteAddr)
			if err != nil {
				return err
			}
			defer out.Close()

			wg := &sync.WaitGroup{}

			wg.Add(2)

			go func() {
				defer wg.Done()
				io.Copy(in, out)
			}()

			go func() {
				defer wg.Done()
				io.Copy(out, in)
			}()
			wg.Wait()

			return nil
		}()
	}
}
