//go:build integration
// +build integration

package preflight

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

// TODO: list of things to test
// - App scope is properly determined
// - Org scope is properly determined
// - App identified by flag is prioritized over the fly.toml file, because it's more visible

func TestTokensListDeterminesScopeApp(t *testing.T) {
	// Get library from preflight test lib using env variables form  .direnv/preflight
	f := testlib.NewTestEnvFromEnv(t)
	// No need to run this on alternate sizes as it only tests the generated config.
	if f.VMSize != "" {
		t.Skip()
	}

	// Create fly.toml in current directory for the purpose of testing --config purposes
	appName := f.CreateRandomAppName()
	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
		f.OrgSlug(), appName, f.PrimaryRegion(),
	)

	// List of commands to verify scope app is selected for
	commandList := [9](string){
		// default, no flags
		"tokens list",
		// --app flag only
		"tokens list -a " + appName,
		// --config flag only
		"tokens list -c fly.toml",
		// --app flag, with --org flag
		"tokens list -a " + appName + " -o " + f.OrgSlug(),
		// --config flag, with --org flag
		"tokens list -c fly.toml -o " + f.OrgSlug(),
		// --scope is app
		"tokens list -s app -a " + appName,
		// --scope is app, with --org flag
		"tokens list -s app -a " + appName + " -o " + f.OrgSlug(),
		// --scope is org, but --config flag added
		"tokens list -s app -c fly.toml -o " + f.OrgSlug(),
		// --scope is org, but --app flag added
		"tokens list -s org " + f.OrgSlug() + " -a " + appName,
	}

	for _, cmd_ := range commandList {
		requireTokensListContains(t, f, cmd_, `Tokens for app`)
	}
}

func TestTokensListDeterminesScopeOrg(t *testing.T) {
	// Get library from preflight test lib using env variables form  .direnv/preflight
	f := testlib.NewTestEnvFromEnv(t)
	// No need to run this on alternate sizes as it only tests the generated config.
	if f.VMSize != "" {
		t.Skip()
	}

	// List of commands to verify scope org is selected for
	commandList := [2](string){
		// --org flag alone
		"tokens list --org " + f.OrgSlug(),
		// --scope org
		"tokens list --scope org " + f.OrgSlug(),
	}

	for _, cmd_ := range commandList {
		console_out := f.Fly(cmd_).StdOutString()
		require.Contains(f, console_out, `Tokens for organization`)
	}
}

func TestTokensListPrioritizesAppFromFlagOverToml(t *testing.T) {
	// Get library from preflight test lib using env variables form  .direnv/preflight
	f := testlib.NewTestEnvFromEnv(t)
	// No need to run this on alternate sizes as it only tests the generated config.
	if f.VMSize != "" {
		t.Skip()
	}

	// Create fly.toml in current directory
	// Get this appName for verifying that the flag is prioritized over toml appname content
	appNameA := f.CreateRandomAppName()
	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
		f.OrgSlug(), appNameA, f.PrimaryRegion(),
	)

	// Delete this first app's toml file, so we can create a new one
	os.Remove(f.WorkDir() + "/fly.toml")
	appNameB := f.CreateRandomAppName()
	f.Fly(
		"launch --now --org %s --name %s --region %s --image nginx --internal-port 80 --ha=false",
		f.OrgSlug(), appNameB, f.PrimaryRegion(),
	)
	// By default use the app name found in the current dir should be used ( appNameB )
	requireTokensListContains(t, f, "tokens list", `Tokens for app "`+appNameB+`"`)

	// But, the app name passed in --app should be prioritized over the toml appname ( appNameA )
	requireTokensListContains(t, f, "tokens list --app "+appNameA, `Tokens for app "`+appNameA+`"`)
}

// requireTokensListContains runs a tokens list command until its output
// contains want. The apps these tests list tokens for were created moments
// earlier, and the API can briefly answer that the app "does not belong to
// selected organization" before the new app has propagated.
func requireTokensListContains(t *testing.T, f *testlib.FlyctlTestEnv, cmd, want string) {
	t.Helper()

	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Contains(c, f.Collect(c).Fly(cmd).StdOutString(), want)
	}, 30*time.Second, 2*time.Second, "%s never printed %q", cmd, want)
}
