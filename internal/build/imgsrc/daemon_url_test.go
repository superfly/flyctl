package imgsrc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuilderAPIURL(t *testing.T) {
	tests := []struct {
		name       string
		daemonHost string
		path       string
		want       string
	}{
		{
			name:       "ipv4",
			daemonHost: "tcp://192.168.1.10:2375",
			path:       "/flyio/v1/extendDeadline",
			want:       "http://192.168.1.10:8080/flyio/v1/extendDeadline",
		},
		{
			name:       "ipv6",
			daemonHost: "tcp://[fdaa:49:2bd7::2]:2375",
			path:       "/flyio/v1/extendDeadline",
			want:       "http://[fdaa:49:2bd7::2]:8080/flyio/v1/extendDeadline",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := builderAPIURL(tc.daemonHost, tc.path)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBuilderAPIURLError(t *testing.T) {
	tests := []struct {
		name       string
		daemonHost string
	}{
		{
			name:       "missing host",
			daemonHost: "unix:///var/run/docker.sock",
		},
		{
			name:       "invalid url",
			daemonHost: "://bad-url",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := builderAPIURL(tc.daemonHost, "/flyio/v1/extendDeadline")
			require.Error(t, err)
		})
	}
}
