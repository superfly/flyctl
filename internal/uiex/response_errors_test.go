package uiex

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponseDecodingDoesNotRecommendRetry(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		call   func(context.Context, *Client) error
	}{
		{"create build", 200, func(ctx context.Context, c *Client) error {
			_, err := c.CreateBuild(ctx, CreateBuildRequest{})
			return err
		}},
		{"finish build", 200, func(ctx context.Context, c *Client) error {
			_, err := c.FinishBuild(ctx, FinishBuildRequest{})
			return err
		}},
		{"ensure depot", 200, func(ctx context.Context, c *Client) error {
			_, err := c.EnsureDepotBuilder(ctx, EnsureDepotBuilderRequest{})
			return err
		}},
		{"create builder", 201, func(ctx context.Context, c *Client) error {
			_, err := c.CreateFlyManagedBuilder(ctx, "org", "iad")
			return err
		}},
		{"feedback", 201, func(ctx context.Context, c *Client) error {
			_, err := c.SubmitFeedback(ctx, FeedbackRequest{})
			return err
		}},
		{"launch checks", 200, func(ctx context.Context, c *Client) error { _, err := c.GetLaunchChecks(ctx, "org"); return err }},
		{"list organizations", 200, func(ctx context.Context, c *Client) error { _, err := c.ListOrganizations(ctx, false); return err }},
		{"get organization", 200, func(ctx context.Context, c *Client) error { _, err := c.GetOrganization(ctx, "org"); return err }},
		{"release timestamps", 200, func(ctx context.Context, c *Client) error {
			_, err := c.GetAllAppsCurrentReleaseTimestamps(ctx, "org")
			return err
		}},
		{"list releases", 200, func(ctx context.Context, c *Client) error { _, err := c.ListReleases(ctx, "app", 1); return err }},
		{"current release", 200, func(ctx context.Context, c *Client) error { _, err := c.GetCurrentRelease(ctx, "app"); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, body := range []string{"{}", "{"} {
				var calls atomic.Int32
				ctx, client := newLaunchChecksTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.WriteHeader(tc.status)
					_, _ = w.Write([]byte(body))
				})
				err := tc.call(ctx, client)
				if body == "{}" {
					require.NoError(t, err)
				} else {
					var syntaxErr *json.SyntaxError
					require.ErrorAs(t, err, &syntaxErr)
					require.ErrorContains(t, err, "failed to decode response")
					require.NotContains(t, err.Error(), "try again")
				}
				require.Equal(t, int32(1), calls.Load(), "a decoding failure must not repeat the request")
			}
		})
	}
}
