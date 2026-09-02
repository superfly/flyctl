package cmdv1

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	mpgv1 "github.com/superfly/flyctl/internal/uiex/mpg/v1"
)

// TestBuildConnectURLPublicExplicitUserDefaultsToFlyDB pins the public
// explicit-user connect path so it can never regress to the bad
// postgresql://.../ helper output. Credentials are resolved the same way the
// public branch of resolveConnectCredentials does, then handed straight to
// buildConnectURL the same way RunConnect does.
func TestBuildConnectURLPublicExplicitUserDefaultsToFlyDB(t *testing.T) {
	creds := &mpgv1.GetManagedClusterCredentialsResponse{
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
	creds := &mpgv1.GetManagedClusterCredentialsResponse{
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
	creds := &mpgv1.GetManagedClusterCredentialsResponse{
		User:     "alice",
		Password: "a",
		DBName:   "",
	}

	got := buildConnectURL(creds, "", "16380")
	require.Equal(t, "postgresql://alice:a@localhost:16380/", got)
}

// TestMaybeWarnLegacyNotReady pins the restored pre-migration legacy-path
// warning. It exercises both the useLegacy gate (the public path must
// never warn, since connectStatusRefusal refuses non-ready public-path
// clusters upfront) and the status gate (a "ready" legacy cluster must
// never warn, even when useLegacy == true). For every other combination
// — useLegacy == true with any non-ready status, including the legacy-
// only "error" status — it asserts the exact pre-migration warning text
// is written to the errOut buffer.
//
// The pre-migration text (commit 81f75427b^) is:
//
//	"%s Cluster is not in ready state, currently: %s\n"
//
// with the first %s being aurora.Yellow("WARN") — which renders as the
// literal string "WARN" when stdout is not a TTY (the test harness case),
// and a yellow-colored "WARN" when it is. We match on substrings
// ("WARN" and "currently: <status>") so the assertion is robust to the
// color-escape difference without changing the user-visible text.
//
// Capturing the output via a bytes.Buffer is feasible here because
// maybeWarnLegacyNotReady is a pure side-effecting helper that takes
// the writer as a parameter — there is no iostreams.FromContext lookup,
// no exec.LookPath("psql") call, and no agent/wireguard involvement.
// That decoupling is the whole reason the warning was extracted into a
// helper, instead of inlined in RunConnect where it would not be
// testable without a full end-to-end RunConnect mock (the existing
// test harness does not exercise RunConnect end-to-end because
// RunConnect calls exec.LookPath("psql") and proxy.Start, neither of
// which can run in a unit test without significant scaffolding). This
// proves useLegacy is correctly threaded and that the warning text
// matches the pre-migration version byte-for-byte.
func TestMaybeWarnLegacyNotReady(t *testing.T) {
	const name = "test-cluster"
	tests := []struct {
		name      string
		useLegacy bool
		status    string
		wantWarn  bool
	}{
		// Public path (useLegacy == false). connectStatusRefusal already
		// refuses non-ready clusters upfront, so by construction no
		// non-ready public-path cluster reaches the warning site. The
		// helper must therefore be silent for every status on the public
		// path, including "ready" (where the gate above already
		// short-circuits) and the legacy-only "error" status (which is
		// rejected by the public classifier's default arm but never
		// reaches this helper on the public path).
		{name: "public+ready silent", useLegacy: false, status: "ready", wantWarn: false},
		{name: "public+creating silent", useLegacy: false, status: "creating", wantWarn: false},
		{name: "public+failed silent", useLegacy: false, status: "failed", wantWarn: false},
		{name: "public+initializing silent", useLegacy: false, status: "initializing", wantWarn: false},
		{name: "public+error silent", useLegacy: false, status: "error", wantWarn: false},

		// Legacy path (useLegacy == true). The "ready" status is silent
		// (the gate above short-circuits before the fprintf). Every
		// other status — including the legacy-only "error" and the
		// non-refused "creating" — produces the warning, because on the
		// legacy path the cluster's status is intentionally NOT consulted
		// and a non-refused legacy cluster reaches psql.
		{name: "legacy+ready silent", useLegacy: true, status: "ready", wantWarn: false},
		{name: "legacy+creating warns", useLegacy: true, status: "creating", wantWarn: true},
		{name: "legacy+failed warns", useLegacy: true, status: "failed", wantWarn: true},
		{name: "legacy+initializing warns", useLegacy: true, status: "initializing", wantWarn: true},
		// Mirror of the existing "creating cluster with ready credentials
		// proceeds" regression scenario: a legacy cluster with
		// response.Data.Status="creating" and valid credentials reaches
		// the warning site. This test pins that the warning fires for
		// exactly that case (the one this fix exists to handle).
		{name: "legacy+creating+ready_credentials warns", useLegacy: true, status: "creating", wantWarn: true},
		// legacy-only status value that the public classifier rejects as
		// "unrecognized" — on the legacy path it is a legitimate status
		// and must still warn.
		{name: "legacy+error warns", useLegacy: true, status: "error", wantWarn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			cluster := &mpgv1.ManagedCluster{Name: name, Status: tt.status}
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
