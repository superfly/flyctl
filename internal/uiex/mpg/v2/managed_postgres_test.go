package v2

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/superfly/fly-go/tokens"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/uiex"
	"github.com/superfly/flyctl/iostreams"
)

func setupTestClient(server *httptest.Server) (*Client, context.Context, error) {
	baseURL, err := url.Parse(server.URL)
	if err != nil {
		return nil, nil, err
	}

	ctx := context.Background()
	ios, _, _, _ := iostreams.Test()
	ctx = iostreams.NewContext(ctx, ios)

	ctx = config.NewContext(ctx, &config.Config{
		Tokens: &tokens.Tokens{},
	})

	client, err := NewClientWithOptions(ctx, uiex.NewClientOpts{BaseURL: baseURL})
	if err != nil {
		return nil, nil, err
	}

	return client, ctx, nil
}

// TestRestoreClusterBackup_NamePayload asserts the wire format of the optional
// destination name: omitted entirely when unset, so an older ui-ex deployment
// sees exactly the payload it saw before this flag existed.
func TestRestoreClusterBackup_NamePayload(t *testing.T) {
	tests := []struct {
		desc     string
		name     string
		wantName any
	}{
		{desc: "unset name is omitted", name: "", wantName: nil},
		{desc: "explicit name is sent", name: "my-restore", wantName: "my-restore"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			var body map[string]any

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("expected POST request, got %s", r.Method)
				}
				if r.URL.Path != "/api/v1/postgresv2/test-cluster-id/restore" {
					t.Errorf("expected path /api/v1/postgresv2/test-cluster-id/restore, got %s", r.URL.Path)
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("failed to decode request body: %v", err)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(RestoreClusterBackupResponse{})
			}))
			defer server.Close()

			client, ctx, err := setupTestClient(server)
			if err != nil {
				t.Fatalf("failed to setup test client: %v", err)
			}

			_, err = client.RestoreClusterBackup(ctx, "test-cluster-id", RestoreClusterBackupInput{
				BackupId: "backup-1",
				Name:     tt.name,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got := body["backup_id"]; got != "backup-1" {
				t.Errorf("expected backup_id backup-1, got %v", got)
			}

			got, present := body["name"]
			if tt.wantName == nil {
				if present {
					t.Errorf("expected name to be omitted, got %v", got)
				}
			} else if got != tt.wantName {
				t.Errorf("expected name %v, got %v", tt.wantName, got)
			}
		})
	}
}
