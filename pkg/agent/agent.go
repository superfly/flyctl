package agent

import (
	"net"
	"os"
	"path/filepath"
	"strings"
)

func IsIPv6(addr string) bool {
	addr = strings.Trim(addr, "[]")
	ip := net.ParseIP(addr)
	return ip != nil && ip.To16() != nil
}

// TODO: deprecate
func PathToSocket() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}

	return filepath.Join(dir, ".fly", "fly-agent.sock")
}

type Instances struct {
	Labels    []string
	Addresses []string
}
