package flypg

// Run: go test ./flypg   (repo root) or cd flypg && go test .
//
// Do not run go test flypg/http_test.go — that compiles only this file, so
// NewFromInstance and flypgPort from http.go are undefined.

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatPGBaseURL_IPv4(t *testing.T) {
	client := NewFromInstance("192.168.1.1", nil)
	assert.Equal(t, "http://192.168.1.1:5500", client.BaseURL)
	assertParsableFlyPGURL(t, client.BaseURL, "192.168.1.1", "5500")
}

func TestFormatPGBaseURL_IPv6(t *testing.T) {
	client := NewFromInstance("fdaa:2:be18:a7b:620:7ff8:9a20:2", nil)
	assert.Equal(t, "http://[fdaa:2:be18:a7b:620:7ff8:9a20:2]:5500", client.BaseURL)
	assertParsableFlyPGURL(t, client.BaseURL, "fdaa:2:be18:a7b:620:7ff8:9a20:2", "5500")
}

func TestFormatPGBaseURL_Hostname(t *testing.T) {
	client := NewFromInstance("myapp.internal", nil)
	assert.Equal(t, "http://myapp.internal:5500", client.BaseURL)
	assertParsableFlyPGURL(t, client.BaseURL, "myapp.internal", "5500")
}

func TestFormatPGBaseURL_Table(t *testing.T) {
	port := flypgPort
	tests := []struct {
		name         string
		address      string
		wantBaseURL  string
		wantHostname string
	}{
		{
			name:         "ipv4",
			address:      "10.0.0.1",
			wantBaseURL:  "http://10.0.0.1:5500",
			wantHostname: "10.0.0.1",
		},
		{
			name:         "ipv6_compressed",
			address:      "2001:db8::1",
			wantBaseURL:  "http://[2001:db8::1]:5500",
			wantHostname: "2001:db8::1",
		},
		{
			name:         "hostname",
			address:      "pg.example.internal",
			wantBaseURL:  "http://pg.example.internal:5500",
			wantHostname: "pg.example.internal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewFromInstance(tt.address, nil).BaseURL
			assert.Equal(t, tt.wantBaseURL, got)
			assertParsableFlyPGURL(t, got, tt.wantHostname, port)
		})
	}
}

// assertParsableFlyPGURL checks the base URL parses and Hostname/Port match expectations
// (same rules as net/http for dialing).
func assertParsableFlyPGURL(t *testing.T, raw, wantHostname, wantPort string) {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "http", u.Scheme)
	assert.Equal(t, wantHostname, u.Hostname())
	assert.Equal(t, wantPort, u.Port())
}
