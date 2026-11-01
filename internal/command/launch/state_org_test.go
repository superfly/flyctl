package launch

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/internal/command/launch/plan"
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

func TestSetOrgIDIgnoresUnknownOrg(t *testing.T) {
	state := &launchState{}

	state.setOrgID(nil)
	state.setOrgID(&uiex.Organization{Slug: "personal"})

	assert.Empty(t, state.orgID)
}
