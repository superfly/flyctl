package uiex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
)

func newLaunchChecksTestClient(t *testing.T, handler http.HandlerFunc) (context.Context, *Client) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	baseURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	ctx := config.NewContext(context.Background(), &config.Config{Tokens: &tokens.Tokens{}})
	client, err := NewWithOptions(ctx, NewClientOpts{BaseURL: baseURL})
	require.NoError(t, err)

	return ctx, client
}

func TestGetLaunchChecks(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/organizations/acme/launch-checks", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"can_launch":true,"checks":{
			"payment_method":{"passed":true},
			"high_risk":{"passed":true},
			"machine_quota":{"passed":true,"current":85,"limit":100}
		}}}`))
	})

	checks, err := client.GetLaunchChecks(ctx, "acme")
	require.NoError(t, err)

	assert.True(t, checks.CanLaunch)
	quota := checks.Checks.MachineQuota
	assert.True(t, quota.Passed)
	assert.Equal(t, 85, quota.Current)
	require.NotNil(t, quota.Limit)
	assert.Equal(t, 100, *quota.Limit)
}

func TestGetLaunchChecksWithoutLimit(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"can_launch":true,"checks":{"machine_quota":{"passed":true,"current":3,"limit":null}}}}`))
	})

	checks, err := client.GetLaunchChecks(ctx, "acme")
	require.NoError(t, err)

	assert.Equal(t, 3, checks.Checks.MachineQuota.Current)
	assert.Nil(t, checks.Checks.MachineQuota.Limit)
}

func TestGetLaunchChecksError(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":{"detail":"Forbidden"}}`))
	})

	_, err := client.GetLaunchChecks(ctx, "acme")

	var statusErr *StatusError
	require.ErrorAs(t, err, &statusErr)
	assert.Equal(t, http.StatusForbidden, statusErr.StatusCode)
}
