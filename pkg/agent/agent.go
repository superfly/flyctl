package agent

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
)

// IsIPv6 determines whether a string contains an IPv6 address, including strings in the [host]:port format
func IsIPv6(addr string) bool {
	parts := strings.Split(addr, ":")

	if len(parts) > 1 {
		addr = parts[0]
	}

	addr = strings.Trim(addr, "[]")
	fmt.Println(addr)
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
