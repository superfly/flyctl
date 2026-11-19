package launch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/command/launch/plan"
	"github.com/superfly/flyctl/internal/metrics"
	"github.com/superfly/flyctl/internal/uiex"
)

func TestOrgRecordsOrgIDForMetrics(t *testing.T) {
	state := &launchState{
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{OrgSlug: "personal"}},
		cache: map[string]any{
			"org,personal": &uiex.Organization{Slug: "personal", InternalNumericID: 4242},
		},
	}

	assert.Empty(t, state.orgID)

	org, err := state.Org(context.Background())
	require.NoError(t, err)
	assert.Equal(t, uint64(4242), org.InternalNumericID)
	assert.Equal(t, "4242", state.orgID)
}

// Launch metrics record the app name before the launch runs; the web UI or a
// taken derived name can change it, and the metrics must report the app that
// was created, next to its ID.
func TestReportToRecordsTheFinalAppName(t *testing.T) {
	status := metrics.LaunchStatusPayload{AppName: "my-app"}
	state := &launchState{
		LaunchManifest: LaunchManifest{Plan: &plan.LaunchPlan{AppName: "my-app-quiet-sun-1234"}},
		appID:          "77",
		orgID:          "4242",
	}

	assert.True(t, state.reportTo(&status))
	assert.Equal(t, "my-app-quiet-sun-1234", status.AppName)
	assert.Equal(t, "77", status.AppID)
	assert.Equal(t, "4242", status.OrgID)

	assert.False(t, state.reportTo(&status), "unchanged name")

	var nilState *launchState
	assert.False(t, nilState.reportTo(&status))
	assert.Equal(t, "my-app-quiet-sun-1234", status.AppName)
}
