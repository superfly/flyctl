package preflight

import (
	
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

// TODO: list of things to test
// - App scope is properly determined
// - Org scope is properly determined
// - App identified by flag is prioritized over the fly.toml file, because it's more visible

func TestTokensListDeterminesScopeApp(t *testing.T){
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

	commandList := [9](string){
		// default, no flags
		"tokens list", 
		// --app flag only
		"tokens list -a "+appName, 
		// --config flag only
		"tokens list -c fly.toml",
		// --app flag, with --org flag 
		"tokens list -a "+appName+" -o kathryn-tan",
		// --config flag, with --org flag 
		"tokens list -c fly.toml -o kathryn-tan",
		// --scope is app 
		"tokens list -s app -a "+appName,
		// --scope is app, with --org flag
		"tokens list -s app -a "+appName+" -o kathryn-tan",
		// --scope is org, but --config flag added
		"tokens list -s app -c fly.toml -o kathryn-tan",
		// --scope is org, but --app flag added
		"tokens list -s org kathryn-tan -a "+appName,
	}

	for _, cmd_:= range commandList {
		console_out := f.Fly(cmd_).StdOutString()
		require.Contains(f, console_out, `Tokens for app`)
	}
}


func TestTokensListDeterminesScopeOrg(t *testing.T){
	// Get library from preflight test lib using env variables form  .direnv/preflight
	f := testlib.NewTestEnvFromEnv(t)
	// No need to run this on alternate sizes as it only tests the generated config.
	if f.VMSize != "" {
		t.Skip()
	}

	commandList := [2](string){
		// --org flag alone
		"tokens list --org kathryn-tan", 
		// --scope org
		"tokens list --scope org kathryn-tan",
	}

	for _, cmd_:= range commandList {
		console_out := f.Fly(cmd_).StdOutString()
		require.Contains(f, console_out, `Tokens for organization`)
	}
}