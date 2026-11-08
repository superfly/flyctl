package command

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/flag"
)

func TestParsePortFlag(t *testing.T) {
	t.Run("single port", func(t *testing.T) {
		internalPort, proto, port, start, end, handlers, err := parsePortFlag("443:8080/tcp:tls:http")
		require.NoError(t, err)
		assert.Equal(t, 8080, internalPort)
		assert.Equal(t, "tcp", proto)
		require.NotNil(t, port)
		assert.Equal(t, 443, *port)
		assert.Nil(t, start)
		assert.Nil(t, end)
		assert.Equal(t, []string{"tls", "http"}, handlers)
	})

	t.Run("port range", func(t *testing.T) {
		internalPort, proto, port, start, end, _, err := parsePortFlag("8000-8010:80/udp")
		require.NoError(t, err)
		assert.Equal(t, 80, internalPort)
		assert.Equal(t, "udp", proto)
		assert.Nil(t, port)
		require.NotNil(t, start)
		require.NotNil(t, end)
		assert.Equal(t, 8000, *start)
		assert.Equal(t, 8010, *end)
	})

	t.Run("invalid end of port range", func(t *testing.T) {
		_, _, _, _, _, _, err := parsePortFlag("8000-abc:80")
		assert.ErrorContains(t, err, "invalid end port")
	})
}

func TestParsePortFlagValidFormats(t *testing.T) {
	for _, tt := range []struct {
		input    string
		internal int
		protocol string
		port     *int
		start    *int
		end      *int
		handlers []string
	}{
		{input: "80", internal: 80, protocol: "tcp", port: new(80)},
		{input: "443/tcp:tls:http", internal: 443, protocol: "tcp", port: new(443), handlers: []string{"tls", "http"}},
		{input: "53/udp", internal: 53, protocol: "udp", port: new(53)},
		{input: "443:8080", internal: 8080, protocol: "tcp", port: new(443)},
		{input: "443:8080/tcp:tls:http", internal: 8080, protocol: "tcp", port: new(443), handlers: []string{"tls", "http"}},
		{input: "8000-8010:80", internal: 80, protocol: "tcp", start: new(8000), end: new(8010)},
		{input: "8000-8010:80/tcp", internal: 80, protocol: "tcp", start: new(8000), end: new(8010)},
		{input: "8000-8010:80/udp", internal: 80, protocol: "udp", start: new(8000), end: new(8010)},
		{input: "8000-8010:8080/tcp:tls:http", internal: 8080, protocol: "tcp", start: new(8000), end: new(8010), handlers: []string{"tls", "http"}},
		{input: "8000-8001:80", internal: 80, protocol: "tcp", start: new(8000), end: new(8001)},
		{input: "8000-8000:80", internal: 80, protocol: "tcp", start: new(8000), end: new(8000)},
		{input: "1-65535:8080", internal: 8080, protocol: "tcp", start: new(1), end: new(65535)},
		{input: "65534-65535:65535", internal: 65535, protocol: "tcp", start: new(65534), end: new(65535)},
		{input: "8000-8010:0", internal: 8000, protocol: "tcp", start: new(8000), end: new(8010)},
	} {
		t.Run(tt.input, func(t *testing.T) {
			internal, protocol, port, start, end, handlers, err := parsePortFlag(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.internal, internal)
			assert.Equal(t, tt.protocol, protocol)
			assert.Equal(t, tt.port, port)
			assert.Equal(t, tt.start, start)
			assert.Equal(t, tt.end, end)
			assert.Equal(t, tt.handlers, handlers)
		})
	}
}

func TestParsePortFlagInvalidFormats(t *testing.T) {
	for _, tt := range []struct {
		input string
		error string
	}{
		{input: "", error: "invalid port"},
		{input: "abc", error: "invalid port"},
		{input: "abc:80", error: "invalid external port"},
		{input: "8000-8010:", error: "invalid machine (internal) port"},
		{input: "8000-8010:abc", error: "invalid machine (internal) port"},
		{input: "abc-8010:80", error: "invalid start port"},
		{input: "-8010:80", error: "invalid start port"},
		{input: "8000-abc:80", error: "invalid end port"},
		{input: "8000-:80", error: "invalid end port"},
		{input: "8000-80.5:80", error: "invalid end port"},
		{input: "8000-999999999999999999999999999:80", error: "invalid end port"},
		{input: "999999999999999999999999999-8010:80", error: "invalid start port"},
		{input: "8000-8010-8020:80", error: "external port must be at most 2 elements"},
		{input: "8000-8010:80:90", error: "port definition must be at most 2 elements"},
		{input: "8000-8010:80/tcp/udp", error: "port must be at most two elements"},
	} {
		t.Run(tt.input, func(t *testing.T) {
			_, _, _, _, _, _, err := parsePortFlag(tt.input)
			require.ErrorContains(t, err, tt.error)
		})
	}
}

func portFlagsContext(t *testing.T, ports ...string) context.Context {
	t.Helper()
	flags := pflag.NewFlagSet("ports", pflag.ContinueOnError)
	flags.StringSliceP("port", "p", nil, "")
	var args []string
	for _, port := range ports {
		args = append(args, "--port", port)
	}
	require.NoError(t, flags.Parse(args))

	return flag.NewContext(context.Background(), flags)
}

func TestDetermineServicesPortRange(t *testing.T) {
	for _, tt := range []struct {
		name     string
		start    int
		end      int
		internal int
		protocol string
		handlers []string
		suffix   string
	}{
		{name: "default TCP", start: 8000, end: 8010, internal: 80, protocol: "tcp"},
		{name: "explicit TCP", start: 8000, end: 8010, internal: 80, protocol: "tcp", suffix: "/tcp"},
		{name: "UDP", start: 8000, end: 8010, internal: 80, protocol: "udp", suffix: "/udp"},
		{name: "handlers", start: 8000, end: 8010, internal: 8080, protocol: "tcp", suffix: "/tcp:tls:http", handlers: []string{"tls", "http"}},
		{name: "single port range", start: 8000, end: 8000, internal: 80, protocol: "tcp"},
		{name: "full port range", start: 1, end: 65535, internal: 8080, protocol: "tcp"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := portFlagsContext(t, fmt.Sprintf("%d-%d:%d%s", tt.start, tt.end, tt.internal, tt.suffix))
			services, err := DetermineServices(ctx, nil)
			require.NoError(t, err)
			require.Len(t, services, 1)
			assert.Equal(t, tt.internal, services[0].InternalPort)
			assert.Equal(t, tt.protocol, services[0].Protocol)
			require.Len(t, services[0].Ports, 1)
			port := services[0].Ports[0]
			assert.Nil(t, port.Port)
			assert.Equal(t, new(tt.start), port.StartPort)
			assert.Equal(t, new(tt.end), port.EndPort)
			assert.Equal(t, tt.handlers, port.Handlers)
			assert.True(t, port.ContainsPort(tt.start))
			assert.True(t, port.ContainsPort(tt.end))
			assert.False(t, port.ContainsPort(tt.start-1))
			assert.False(t, port.ContainsPort(tt.end+1))

			// Check the actual API field names and values, including omitting port.
			encoded, err := json.Marshal(port)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(encoded, &wire))
			assert.Equal(t, float64(tt.start), wire["start_port"])
			assert.Equal(t, float64(tt.end), wire["end_port"])
			assert.NotContains(t, wire, "port")
		})
	}
}

func TestDetermineServicesUpdatePortRange(t *testing.T) {
	for _, end := range []int{8010, 8001, 8000} {
		t.Run(fmt.Sprint(end), func(t *testing.T) {
			services := []fly.MachineService{
				{InternalPort: 80, Protocol: "tcp", Ports: []fly.MachinePort{
					{StartPort: new(8000), EndPort: new(8005), ForceHTTPS: true},
					{Port: new(443), Handlers: []string{"tls", "http"}},
				}},
				{InternalPort: 80, Protocol: "udp", Ports: []fly.MachinePort{{StartPort: new(8000), EndPort: new(8020)}}},
			}
			ctx := portFlagsContext(t, fmt.Sprintf("8000-%d:80/tcp:http", end))
			got, err := DetermineServices(ctx, services)
			require.NoError(t, err)
			require.Len(t, got, 2)
			require.Len(t, got[0].Ports, 2)
			assert.Equal(t, fly.MachinePort{StartPort: new(8000), EndPort: new(end), ForceHTTPS: true, Handlers: []string{"http"}}, got[0].Ports[0])
			assert.Equal(t, fly.MachinePort{Port: new(443), Handlers: []string{"tls", "http"}}, got[0].Ports[1])
			assert.Equal(t, fly.MachinePort{StartPort: new(8000), EndPort: new(8020)}, got[1].Ports[0])
		})
	}
}

func TestDetermineServicesMultiplePortRanges(t *testing.T) {
	ctx := portFlagsContext(t, "8000-8010:80", "9000-9010:80", "443:80/tcp:tls:http", "7000-7010:80/udp", "8000-8020:81")
	got, err := DetermineServices(ctx, nil)
	require.NoError(t, err)
	assert.Equal(t, []fly.MachineService{
		{InternalPort: 80, Protocol: "tcp", Ports: []fly.MachinePort{
			{StartPort: new(8000), EndPort: new(8010)},
			{StartPort: new(9000), EndPort: new(9010)},
			{Port: new(443), Handlers: []string{"tls", "http"}},
		}},
		{InternalPort: 80, Protocol: "udp", Ports: []fly.MachinePort{{StartPort: new(7000), EndPort: new(7010)}}},
		{InternalPort: 81, Protocol: "tcp", Ports: []fly.MachinePort{{StartPort: new(8000), EndPort: new(8020)}}},
	}, got)
}

func TestDetermineServicesRepeatedPortRange(t *testing.T) {
	ctx := portFlagsContext(t, "8000-8010:80/tcp:tls", "8000-8020:80/tcp:http")
	got, err := DetermineServices(ctx, nil)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, []fly.MachinePort{{StartPort: new(8000), EndPort: new(8020), Handlers: []string{"http"}}}, got[0].Ports)
}

func TestDetermineServicesRejectsMalformedRange(t *testing.T) {
	for _, input := range []string{"abc-8010:80", "8000-abc:80", "8000-:80", "8000-999999999999999999999999999:80"} {
		t.Run(input, func(t *testing.T) {
			got, err := DetermineServices(portFlagsContext(t, input), nil)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
}

func FuzzParsePortFlagRange(f *testing.F) {
	for _, ports := range [][3]uint16{{8000, 8010, 80}, {1, 65535, 8080}, {65534, 65535, 65535}, {8000, 8000, 80}} {
		f.Add(ports[0], ports[1], ports[2])
	}
	f.Fuzz(func(t *testing.T, start, end, internal uint16) {
		if start == 0 || end == 0 || internal == 0 {
			t.Skip()
		}
		if start > end {
			start, end = end, start
		}
		input := fmt.Sprintf("%d-%d:%d/tcp:tls:http", start, end, internal)
		gotInternal, protocol, port, gotStart, gotEnd, handlers, err := parsePortFlag(input)
		require.NoError(t, err)
		assert.Equal(t, int(internal), gotInternal)
		assert.Equal(t, "tcp", protocol)
		assert.Nil(t, port)
		assert.Equal(t, new(int(start)), gotStart)
		assert.Equal(t, new(int(end)), gotEnd)
		assert.Equal(t, []string{"tls", "http"}, handlers)
	})
}
