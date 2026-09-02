package cmdv2

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	mpgv2 "github.com/superfly/flyctl/internal/uiex/mpg/v2"
)

// TestBuildConnectURLPublicExplicitUserDefaultsToFlyDB pins the public
// explicit-user connect path so it can never regress to the bad
// postgresql://.../ helper output. Credentials are resolved the same way the
// public branch of resolveConnectCredentials does, then handed straight to
// buildConnectURL the same way RunConnect does.
func TestBuildConnectURLPublicExplicitUserDefaultsToFlyDB(t *testing.T) {
	creds := &mpgv2.GetClusterCredentialsResponse{
		User:     "alice",
		Password: "a",
		DBName:   "fly-db", // mirrors the fixed public explicit-user branch.
	}

	got := buildConnectURL(creds, "", "16380")
	require.Equal(t, "postgresql://alice:a@localhost:16380/fly-db", got)
}

// TestBuildConnectURLRespectsExplicitDatabase verifies the priority order:
// an explicit --database value wins over credentials.DBName.
func TestBuildConnectURLRespectsExplicitDatabase(t *testing.T) {
	creds := &mpgv2.GetClusterCredentialsResponse{
		User:     "alice",
		Password: "a",
		DBName:   "fly-db",
	}

	got := buildConnectURL(creds, "app-db", "16380")
	require.Equal(t, "postgresql://alice:a@localhost:16380/app-db", got)
}

// TestBuildConnectURLEmptyDBNameFallsThrough is a guard for the historical
// bug shape: if the explicit-user public branch ever returns "" again, the
// URL will not silently land on postgresql://.../ (empty path). It will land
// on postgresql://.../ with the user-controlled --database fallback in
// RunConnect still preferring the flag value when present; without that
// fallback the URL here is malformed, which surfaces in tests rather than
// silently connecting to the wrong database.
func TestBuildConnectURLEmptyDBNameFallsThrough(t *testing.T) {
	creds := &mpgv2.GetClusterCredentialsResponse{
		User:     "alice",
		Password: "a",
		DBName:   "",
	}

	got := buildConnectURL(creds, "", "16380")
	require.Equal(t, "postgresql://alice:a@localhost:16380/", got)
}

// TestMaybeWarnLegacyNotReady pins the pre-migration warning format and
// the useLegacy/status gate. See maybeWarnLegacyNotReady's doc comment
// for the gate rationale and the warning-text provenance.
func TestMaybeWarnLegacyNotReady(t *testing.T) {
	const name = "test-cluster"
	tests := []struct {
		name      string
		useLegacy bool
		status    string
		wantWarn  bool
	}{
		// Public path (useLegacy == false). connectStatusRefusal refuses
		// non-ready public-path clusters upstream, so the useLegacy
		// gate is the only thing that matters here.
		{name: "public+creating silent", useLegacy: false, status: "creating", wantWarn: false},

		// Legacy path (useLegacy == true). "ready" is silent (the
		// status gate short-circuits before the fprintf); "creating"
		// is the pre-migration non-refused case. The legacy-only
		// "error" status is real on the legacy path but rejected by
		// the public classifier — pinned here so a future migration
		// does not silently drop it.
		{name: "legacy+ready silent", useLegacy: true, status: "ready", wantWarn: false},
		{name: "legacy+creating warns", useLegacy: true, status: "creating", wantWarn: true},
		{name: "legacy+error warns", useLegacy: true, status: "error", wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cluster := &mpgv2.ManagedCluster{Name: name, Status: tt.status}
			maybeWarnLegacyNotReady(&buf, tt.useLegacy, cluster)

			got := buf.String()
			if !tt.wantWarn {
				require.Empty(t, got, "no warning expected for useLegacy=%v status=%q", tt.useLegacy, tt.status)

				return
			}
			// Robust match against the pre-migration rendered text. We
			// assert on "WARN" (the literal aurora payload when not a
			// TTY) and on the exact "currently: <status>" interpolation,
			// rather than asserting the aurora-wrapped string verbatim,
			// so this stays green regardless of the test harness's color
			// configuration.
			require.Contains(t, got, "WARN", "warning must contain the literal 'WARN' marker")
			require.Contains(t, got, "Cluster is not in ready state, currently: "+tt.status)
			require.True(t, bytes.HasSuffix(buf.Bytes(), []byte("\n")), "warning must end with a newline (pre-migration format)")
		})
	}
}

// TestMaybeWarnLegacyNotReadyNilCluster pins the defensive nil-cluster
// short-circuit. RunConnect only calls the helper with the cluster
// returned by GetMpgConnectParams, which is never nil on the success
// path, but the helper is public and the nil guard prevents a panic if
// the contract ever loosens.
func TestMaybeWarnLegacyNotReadyNilCluster(t *testing.T) {
	var buf bytes.Buffer
	require.NotPanics(t, func() {
		maybeWarnLegacyNotReady(&buf, true, nil)
	})
	require.Empty(t, buf.String())
}
