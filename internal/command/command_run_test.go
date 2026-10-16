package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
