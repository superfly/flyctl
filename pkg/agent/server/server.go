// Package server implements the agent's server.
package server

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/sync/errgroup"
)

// ListenAndServe starts a server on the given unix path.
func ListenAndServe(parent context.Context, path string) error {
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	// we don't need to close as server.Serve will.

	srv := http.Server{
		Handler: newRouter(),
	}

	eg, ctx := errgroup.WithContext(parent)

	eg.Go(func() error {
		select {
		case <-parent.Done():
			// parent bailed; shutdown
			cancelCtx, cancel := context.WithTimeout(context.Background(), time.Second<<2)
			defer cancel()

			return srv.Shutdown(cancelCtx)
		case <-ctx.Done():
			// serve has returned
			return nil
		}
	})

	eg.Go(func() error { return srv.Serve(l) })

	return eg.Wait()
}

func newRouter() *httprouter.Router {
	r := httprouter.New()

	r.HandlerFunc(http.MethodGet, "/status", status)

	return r
}
