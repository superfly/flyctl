// Package server implements the agent's server.
package server

import (
	"context"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/sync/errgroup"

	"github.com/superfly/flyctl/pkg/agent/server/internal/state"
)

func Serve(parent context.Context, l net.Listener, logger *log.Logger, daemon bool) error {
	defer logger.Print("exited.")

	eg, ctx := errgroup.WithContext(parent)

	srv := http.Server{
		Handler: newRouter(),
		ConnContext: func(ctx context.Context, _ net.Conn) context.Context {
			ctx = state.WithDaemon(ctx, daemon)
			ctx = state.WithLogger(ctx, logger)

			return ctx
		},
	}

	eg.Go(func() (err error) {
		select {
		case <-ctx.Done():
			logger.Print("shutting down ...")

			// parent context canceled; shut down the server
			cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second>>1)
			defer cancel()

			if err = srv.Shutdown(cancelCtx); err != nil {
				log.Printf("server shutdown: %v", err)
			}
		}

		return
	})

	eg.Go(func() error {
		return srv.Serve(l)
	})

	return eg.Wait()
}

func newRouter() *httprouter.Router {
	r := httprouter.New()

	r.HandlerFunc(http.MethodGet, "/status", status)

	return r
}
