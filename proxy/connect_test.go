package proxy

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestListenAddr(t *testing.T) {
	for bind, want := range map[string]string{
		"":          ":4280",
		"127.0.0.1": "127.0.0.1:4280",
		"0.0.0.0":   "0.0.0.0:4280",
		"::1":       "[::1]:4280",
		"[::1]":     "[::1]:4280", // already worked; keep it working
		"::":        "[::]:4280",
		"localhost": "localhost:4280",
	} {
		got := listenAddr(bind, "4280")
		require.Equal(t, want, got, "bind %q", bind)
		_, err := net.ResolveTCPAddr("tcp", got)
		require.NoError(t, err, "bind %q", bind)
	}
}
