package uiex

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListDeployments(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/api/v1/apps/my-app/deployments", r.URL.Path)
		assert.Equal(t, "5", r.URL.Query().Get("limit"))
		assert.Equal(t, "", r.URL.Query().Get("before"))
		_, _ = w.Write([]byte(`{"data":[{"id":2,"app":"my-app","status":"failed","trigger":"webhook:github",
			"git":{"repository":"acme/shop","commit":"2222bbb"},
			"failure":{"step":"build","category":"npm_build_error","title":"npm run build failed"},
			"created_at":"2026-11-04T15:05:52Z","updated_at":"2026-11-04T15:06:40Z",
			"dashboard_url":"https://fly.io/apps/my-app/deployments/2"}],"page":{"next_before":null}}`))
	})

	deployments, err := client.ListDeployments(ctx, "my-app", 5, 0)
	require.NoError(t, err)
	require.Len(t, deployments, 1)

	d := deployments[0]
	assert.Equal(t, 2, d.ID)
	assert.True(t, d.Finished())
	assert.Equal(t, "2222bbb", *d.Git.Commit)
	assert.Equal(t, "npm_build_error", d.Failure.Category)
}

func TestGetDeploymentWaits(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/apps/my-app/deployments/latest", r.URL.Path)
		assert.Equal(t, "30", r.URL.Query().Get("wait"))
		_, _ = w.Write([]byte(`{"data":{"id":3,"status":"running","current_step":"build",
			"steps":[{"name":"build","description":"Build image","status":"running"}]}}`))
	})

	d, err := client.GetDeployment(ctx, "my-app", "latest", 30)
	require.NoError(t, err)
	assert.False(t, d.Finished())
	assert.Equal(t, "build", *d.CurrentStep)
	assert.Equal(t, "Build image", d.Steps[0].Description)
}

func TestGetDeploymentDiagnosisReturnsMarkdown(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/apps/my-app/deployments/7/diagnosis", r.URL.Path)
		assert.Equal(t, "10", r.URL.Query().Get("lines"))
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# Fly.io deployment 7 of my-app: failed\n"))
	})

	brief, err := client.GetDeploymentDiagnosis(ctx, "my-app", "7", 10)
	require.NoError(t, err)
	assert.Equal(t, "# Fly.io deployment 7 of my-app: failed\n", brief)
}

func TestCreateDeploymentSendsBody(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/apps/my-app/deployments", r.URL.Path)

		body, _ := io.ReadAll(r.Body)
		var got map[string]any
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, map[string]any{"ref": "main", "replace": true}, got)

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":9,"status":"running","trigger":"api"}}`))
	})

	d, err := client.CreateDeployment(ctx, "my-app", CreateDeploymentRequest{Ref: "main", Replace: true})
	require.NoError(t, err)
	assert.Equal(t, 9, d.ID)
}

func TestDeploymentsErrorCarriesMessage(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Deployment 3 is still running."}`))
	})

	_, err := client.RetryDeployment(ctx, "my-app", "3")

	var apiErr *DeploymentsAPIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusConflict, apiErr.StatusCode)
	assert.Equal(t, "Deployment 3 is still running.", err.Error())
}

func TestLaunchActionRequired(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/apps/my-app/launches", r.URL.Path)
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Install the GitHub App.","action_required":
			{"kind":"github_app_install","url":"https://github.com/apps/fly-io/installations/new"}}`))
	})

	_, err := client.LaunchFromRepository(ctx, "my-app", LaunchRequest{Repository: "acme/shop"})

	var apiErr *DeploymentsAPIError
	require.ErrorAs(t, err, &apiErr)
	require.NotNil(t, apiErr.ActionRequired)
	assert.Equal(t, "github_app_install", apiErr.ActionRequired.Kind)
	assert.Contains(t, err.Error(), "https://github.com/apps/fly-io/installations/new")
}

func TestDeploymentsNotFound(t *testing.T) {
	ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":{"detail":"Not Found"}}`))
	})

	_, err := client.GetDeployment(ctx, "my-app", "latest", 0)
	assert.True(t, IsDeploymentsNotFound(err))
	assert.Contains(t, err.Error(), "not found")
}
