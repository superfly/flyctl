package machines

import (
	"net"

	"github.com/superfly/flyctl/api"
)

func IpAddress(machine *api.Machine) string {
	ip := machine.IPs.Nodes[0].IP
	peerIP := net.ParseIP(ip)
	return peerIP.String()
}
