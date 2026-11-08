package uiex

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
)

func newFeedbackTestClient(t *testing.T, handler http.HandlerFunc) (context.Context, *Client) {
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

func TestSubmitFeedback(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		body        string
		wantErr     string
		wantID      string
		wantAlready bool
	}{
		{name: "created", status: http.StatusCreated, body: `{"already_submitted":false,"feedback_id":"abc"}`, wantID: "abc"},
		{name: "already submitted", status: http.StatusOK, body: `{"already_submitted":true,"feedback_id":"abc"}`, wantID: "abc", wantAlready: true},
		{name: "legacy token", status: http.StatusBadRequest, body: `{}`, wantErr: "legacy tokens"},
		{name: "unauthorized", status: http.StatusUnauthorized, body: `{}`, wantErr: "not authorized"},
		{name: "validation", status: http.StatusUnprocessableEntity, body: `{"error":"bad category"}`, wantErr: "bad category"},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{}`, wantErr: "too many feedback reports"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, client := newFeedbackTestClient(t, func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, http.MethodPost, r.Method)
				assert.Equal(t, "/api/v1/feedback", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.True(t, strings.HasPrefix(r.Header.Get("Authorization"), "Bearer"))

				var got FeedbackRequest
				assert.NoError(t, json.NewDecoder(r.Body).Decode(&got))
				assert.Equal(t, FeedbackRequest{Category: "bug", Note: "broken", RequestID: "req-1"}, got)

				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			})

			out, err := client.SubmitFeedback(ctx, FeedbackRequest{Category: "bug", Note: "broken", RequestID: "req-1"})
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantID, out.FeedbackID)
			assert.Equal(t, tt.wantAlready, out.AlreadySubmitted)
		})
	}
}
