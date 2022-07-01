package ip

import (
	"net"
	"strings"
)

func IsV6(addr string) bool {
	addr = strings.Trim(addr, "[]")
	ip := net.ParseIP(addr)
	return ip != nil && ip.To16() != nil
}
