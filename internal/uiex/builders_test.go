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

func newBuildersTestClient(t *testing.T, handler http.HandlerFunc) (context.Context, *Client) {
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

func TestCreateBuild_ErrorMessages(t *testing.T) {
	t.Run("non-200 status error", func(t *testing.T) {
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal depot error"))
		})

		resp, err := client.CreateBuild(ctx, CreateBuildRequest{})
		require.Nil(t, resp)
		require.Error(t, err)
		assert.Equal(t, "create build failed (status 500): internal depot error", err.Error())
		assert.NotContains(t, err.Error(), "ensure depot builder failed")
		assert.NotContains(t, err.Error(), "please try again")
	})

	t.Run("decode error on 200", func(t *testing.T) {
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("invalid json"))
		})

		resp, err := client.CreateBuild(ctx, CreateBuildRequest{})
		require.Nil(t, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response:")
		assert.NotContains(t, err.Error(), "please try again")
	})

	t.Run("success", func(t *testing.T) {
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":123,"status":"started"}`))
		})

		resp, err := client.CreateBuild(ctx, CreateBuildRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int64(123), resp.Id)
		assert.Equal(t, "started", resp.Status)
	})
}

func TestFinishBuild_ErrorMessages(t *testing.T) {
	t.Run("non-200 status error", func(t *testing.T) {
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal depot error"))
		})

		resp, err := client.FinishBuild(ctx, FinishBuildRequest{})
		require.Nil(t, resp)
		require.Error(t, err)
		assert.Equal(t, "finish build failed (status 500): internal depot error", err.Error())
		assert.NotContains(t, err.Error(), "ensure depot builder failed")
		assert.NotContains(t, err.Error(), "create build failed")
		assert.NotContains(t, err.Error(), "please try again")
	})

	t.Run("decode error on 200", func(t *testing.T) {
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("not valid json"))
		})

		resp, err := client.FinishBuild(ctx, FinishBuildRequest{})
		require.Nil(t, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response:")
		assert.NotContains(t, err.Error(), "please try again")
	})

	t.Run("success", func(t *testing.T) {
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":456,"status":"completed"}`))
		})

		resp, err := client.FinishBuild(ctx, FinishBuildRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, int64(456), resp.Id)
		assert.Equal(t, "completed", resp.Status)
	})
}

func TestEnsureDepotBuilder_ErrorMessages(t *testing.T) {
	t.Run("non-200 status error", func(t *testing.T) {
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("internal depot error"))
		})

		resp, err := client.EnsureDepotBuilder(ctx, EnsureDepotBuilderRequest{})
		require.Nil(t, resp)
		require.Error(t, err)
		assert.Equal(t, "ensure depot builder failed (status 500): internal depot error", err.Error())
		assert.NotContains(t, err.Error(), "please try again")
	})

	t.Run("decode error on 200", func(t *testing.T) {
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("malformed json"))
		})

		resp, err := client.EnsureDepotBuilder(ctx, EnsureDepotBuilderRequest{})
		require.Nil(t, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to decode response:")
		assert.NotContains(t, err.Error(), "please try again")
	})

	t.Run("success", func(t *testing.T) {
		buildID := "bld_789"
		ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"build_id":"bld_789"}`))
		})

		resp, err := client.EnsureDepotBuilder(ctx, EnsureDepotBuilderRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		assert.Equal(t, buildID, *resp.BuildId)
	})
}

func TestCreateFlyManagedBuilder_DecodeError(t *testing.T) {
	ctx, client := newBuildersTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("bad json"))
	})

	_, err := client.CreateFlyManagedBuilder(ctx, "acme", "iad")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode response:")
	assert.NotContains(t, err.Error(), "please try again")
}
