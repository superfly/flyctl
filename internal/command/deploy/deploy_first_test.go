package deploy

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/flaps"
	"github.com/superfly/flyctl/internal/appconfig"
	"github.com/superfly/flyctl/internal/mock"
	"github.com/superfly/flyctl/iostreams"
)

func TestProvisionIpsOnFirstDeployPrivateIncludesOrgAndNetwork(t *testing.T) {
	var request flaps.AssignIPRequest
	client := &mock.FlapsClient{
		GetIPAssignmentsFunc: func(context.Context, string) (*flaps.ListIPAssignmentsResponse, error) {
			return &flaps.ListIPAssignmentsResponse{}, nil
		},
		AssignIPFunc: func(_ context.Context, appName string, req flaps.AssignIPRequest) (*flaps.IPAssignment, error) {
			assert.Equal(t, "private-app", appName)
			request = req
			return &flaps.IPAssignment{IP: "fdaa::1"}, nil
		},
	}
	ios, _, _, _ := iostreams.Test()
	md := &machineDeployment{
		app: &flaps.App{
			Name:    "private-app",
			Network: "custom-network",
		},
		appConfig: &appconfig.Config{
			HTTPService: &appconfig.HTTPService{},
		},
		flapsClient:   client,
		io:            ios,
		colorize:      ios.ColorScheme(),
		isFirstDeploy: true,
	}

	err := md.provisionIpsOnFirstDeploy(t.Context(), "private", "private-org")
	require.NoError(t, err)
	assert.Equal(t, flaps.AssignIPRequest{
		Type:         "private_v6",
		Organization: "private-org",
		Network:      "custom-network",
	}, request)
}
