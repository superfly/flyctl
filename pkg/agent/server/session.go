package server

import (
	"context"
	"net"
)

func run(ctx context.Context, conn net.Conn) {

}

type session struct {
	conn net.Conn
}
