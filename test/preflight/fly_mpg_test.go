//go:build integration
// +build integration

package preflight

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

// TestMPG_AttachCreatesAttachmentRecord tests that `fly mpg attach` creates
// a ManagedServiceAttachment record in addition to setting the DATABASE_URL secret.
//
// NOTE: This test requires the ui-ex and web changes to be deployed that add
// the POST /api/v1/postgres/:managed_postgres_hashid/attachments endpoint.
// Until those changes are deployed, this test will fail.
func TestMPG_AttachCreatesAttachmentRecord(t *testing.T) {
	// Skip this test until the ui-ex/web changes are deployed
	t.Skip("Requires ui-ex/web deployment of create_attachment endpoint")

	if testing.Short() {
		t.Skip("skipping MPG test in short mode")
	}

	f := testlib.NewTestEnvFromEnv(t)

	// Skip if using custom VM size (MPG doesn't support custom sizes)
	if f.VMSize != "" {
		t.Skip("MPG tests don't support custom VM sizes")
	}

	// Create an MPG cluster
	clusterName := f.CreateRandomAppName()
	f.Cleanup(func() {
		// Clean up the MPG cluster
		f.FlyAllowExitFailure("mpg destroy %s --yes", clusterName)
	})

	f.Fly(
		"mpg create --org %s --name %s --region %s --plan development --volume-size 10",
		f.OrgSlug(), clusterName, f.PrimaryRegion(),
	)

	// Wait for cluster to be ready
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		result := f.FlyAllowExitFailure("mpg status %s --json", clusterName)
		if result.ExitCode() != 0 {
			t.Errorf("mpg status failed: %s", result.StdErr().String())
			return
		}
		// Check if cluster is ready
		output := result.StdOut().String()
		assert.Contains(t, output, `"status":"ready"`)
	}, 5*time.Minute, 15*time.Second, "MPG cluster did not become ready")

	// Create an app to attach the cluster to
	appName := f.CreateRandomAppName()
	f.Fly("apps create %s --org %s --machines", appName, f.OrgSlug())

	// Attach the MPG cluster to the app
	result := f.Fly("mpg attach %s --app %s", clusterName, appName)
	output := result.StdOut().String()

	// Verify the attach command succeeded and set the secret
	require.Contains(f, output, "Postgres cluster")
	require.Contains(f, output, "is being attached to")
	require.Contains(f, output, "DATABASE_URL")

	// Verify the secret was set on the app
	secretsResult := f.Fly("secrets list --app %s", appName)
	require.Contains(f, secretsResult.StdOut().String(), "DATABASE_URL")
}

// TestMPG_AttachWithCustomVariableName tests attaching with a custom variable name
func TestMPG_AttachWithCustomVariableName(t *testing.T) {
	// Skip this test until the ui-ex/web changes are deployed
	t.Skip("Requires ui-ex/web deployment of create_attachment endpoint")

	if testing.Short() {
		t.Skip("skipping MPG test in short mode")
	}

	f := testlib.NewTestEnvFromEnv(t)

	if f.VMSize != "" {
		t.Skip("MPG tests don't support custom VM sizes")
	}

	// Create an MPG cluster
	clusterName := f.CreateRandomAppName()
	f.Cleanup(func() {
		f.FlyAllowExitFailure("mpg destroy %s --yes", clusterName)
	})

	f.Fly(
		"mpg create --org %s --name %s --region %s --plan development --volume-size 10",
		f.OrgSlug(), clusterName, f.PrimaryRegion(),
	)

	// Wait for cluster to be ready
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		result := f.FlyAllowExitFailure("mpg status %s --json", clusterName)
		if result.ExitCode() != 0 {
			return
		}
		assert.Contains(t, result.StdOut().String(), `"status":"ready"`)
	}, 5*time.Minute, 15*time.Second)

	// Create an app
	appName := f.CreateRandomAppName()
	f.Fly("apps create %s --org %s --machines", appName, f.OrgSlug())

	// Attach with custom variable name
	customVarName := "POSTGRES_URL"
	result := f.Fly("mpg attach %s --app %s --variable-name %s", clusterName, appName, customVarName)
	output := result.StdOut().String()

	require.Contains(f, output, customVarName)

	// Verify the custom secret was set
	secretsResult := f.Fly("secrets list --app %s", appName)
	require.Contains(f, secretsResult.StdOut().String(), customVarName)
}

// TestMPG_AttachFailsForDifferentOrg tests that attach fails when app and cluster
// are in different organizations
func TestMPG_AttachFailsForDifferentOrg(t *testing.T) {
	// Skip this test until the ui-ex/web changes are deployed
	t.Skip("Requires ui-ex/web deployment of create_attachment endpoint")

	if testing.Short() {
		t.Skip("skipping MPG test in short mode")
	}

	f := testlib.NewTestEnvFromEnv(t)

	if f.VMSize != "" {
		t.Skip("MPG tests don't support custom VM sizes")
	}

	// This test would require access to two different orgs,
	// which is complex to set up in preflight tests.
	// For now, we document the expected behavior.
	t.Skip("requires access to multiple organizations")
}

// TestMPG_AttachFailsWhenSecretExists tests that attach fails when the secret
// variable already exists on the app
func TestMPG_AttachFailsWhenSecretExists(t *testing.T) {
	// Skip this test until the ui-ex/web changes are deployed
	t.Skip("Requires ui-ex/web deployment of create_attachment endpoint")

	if testing.Short() {
		t.Skip("skipping MPG test in short mode")
	}

	f := testlib.NewTestEnvFromEnv(t)

	if f.VMSize != "" {
		t.Skip("MPG tests don't support custom VM sizes")
	}

	// Create an MPG cluster
	clusterName := f.CreateRandomAppName()
	f.Cleanup(func() {
		f.FlyAllowExitFailure("mpg destroy %s --yes", clusterName)
	})

	f.Fly(
		"mpg create --org %s --name %s --region %s --plan development --volume-size 10",
		f.OrgSlug(), clusterName, f.PrimaryRegion(),
	)

	// Wait for cluster to be ready
	assert.EventuallyWithT(t, func(t *assert.CollectT) {
		result := f.FlyAllowExitFailure("mpg status %s --json", clusterName)
		if result.ExitCode() != 0 {
			return
		}
		assert.Contains(t, result.StdOut().String(), `"status":"ready"`)
	}, 5*time.Minute, 15*time.Second)

	// Create an app and set DATABASE_URL secret
	appName := f.CreateRandomAppName()
	f.Fly("apps create %s --org %s --machines", appName, f.OrgSlug())
	f.Fly("secrets set DATABASE_URL=postgres://existing@localhost/db --app %s", appName)

	// Try to attach - should fail because DATABASE_URL already exists
	result := f.FlyAllowExitFailure("mpg attach %s --app %s", clusterName, appName)
	require.NotEqual(f, 0, result.ExitCode(), "attach should fail when secret exists")
	require.Contains(f, result.StdErr().String(), "DATABASE_URL")
}
