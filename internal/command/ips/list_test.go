package ips

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/fly-go/flaps"
)

func TestIPAssignmentsToIPAddresses(t *testing.T) {
	now := time.Now()
	svc := "web"
	assignments := []flaps.IPAssignment{
		{IP: "37.16.1.1", Region: "global", CreatedAt: now},
		{IP: "66.241.124.1", Region: "global", Shared: true, CreatedAt: now},
		{IP: "2a09:8280:1::1", Region: "global", CreatedAt: now},
		{IP: "fdaa:0:1::2", Region: "global", ServiceName: svc, CreatedAt: now, Network: &flaps.IPAssignmentNetwork{Name: "my-net", OrgSlug: "my-org"}},
		{IP: "149.248.1.1", Region: "ams", Egress: true, CreatedAt: now},
		{IP: "2a09:8280:2::1", Region: "ams", Egress: true, CreatedAt: now},
	}

	got := ipAssignmentsToIPAddresses(assignments)
	require.Len(t, got, len(assignments))

	wantTypes := []string{"v4", "shared_v4", "v6", "private_v6", "egress_v4", "egress_v6"}
	for i, ip := range got {
		assert.Equal(t, assignments[i].IP, ip.Address)
		assert.Equal(t, wantTypes[i], ip.Type)
		assert.Equal(t, assignments[i].Region, ip.Region)
		assert.Equal(t, now, ip.CreatedAt)
	}
	assert.Equal(t, svc, got[3].ServiceName)

	for i, ip := range got {
		if i == 3 {
			continue
		}
		assert.Nil(t, ip.Network, "ip %d should have no network", i)
	}
	require.NotNil(t, got[3].Network)
	assert.Equal(t, "my-net", got[3].Network.Name)
	require.NotNil(t, got[3].Network.Organization)
	assert.Equal(t, "my-org", got[3].Network.Organization.Slug)
}

func TestNetworkName(t *testing.T) {
	assert.Equal(t, "", networkName(fly.IPAddress{}, "my-org"))

	flycast := ipAssignmentsToIPAddresses([]flaps.IPAssignment{
		{IP: "fdaa:0:1::2", Network: &flaps.IPAssignmentNetwork{Name: "", OrgSlug: "my-org"}},
		{IP: "fdaa:0:2::2", Network: &flaps.IPAssignmentNetwork{Name: "my-net", OrgSlug: "my-org"}},
	})

	// Networks in the app's own org are shown without the org prefix.
	assert.Equal(t, "default", networkName(flycast[0], "my-org"))
	assert.Equal(t, "my-net", networkName(flycast[1], "my-org"))

	// Networks in other orgs (or with an unknown app org) carry the org prefix.
	assert.Equal(t, "my-org/default", networkName(flycast[0], "other-org"))
	assert.Equal(t, "my-org/my-net", networkName(flycast[1], ""))
}

func TestEgressIPAddressesByRegion(t *testing.T) {
	now := time.Now()
	assignments := []flaps.IPAssignment{
		{IP: "37.16.1.1", Region: "global", CreatedAt: now},
		{IP: "149.248.1.1", Region: "ams", Egress: true, CreatedAt: now},
		{IP: "2a09:8280:2::1", Region: "ams", Egress: true, CreatedAt: now},
		{IP: "149.248.2.1", Region: "fra", Egress: true, CreatedAt: now},
	}

	got := egressIPAddressesByRegion(assignments)
	require.Len(t, got, 2)

	require.Len(t, got["ams"], 2)
	assert.Equal(t, "149.248.1.1", got["ams"][0].IP)
	assert.Equal(t, 4, got["ams"][0].Version)
	assert.Equal(t, "ams", got["ams"][0].Region)
	assert.Equal(t, now, got["ams"][0].UpdatedAt)
	assert.Equal(t, "2a09:8280:2::1", got["ams"][1].IP)
	assert.Equal(t, 6, got["ams"][1].Version)

	require.Len(t, got["fra"], 1)
	assert.Equal(t, 4, got["fra"][0].Version)

	assert.Empty(t, egressIPAddressesByRegion(assignments[:1]))
}
