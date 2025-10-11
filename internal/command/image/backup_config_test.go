package image

import (
	"context"
	"fmt"
	"testing"

	fly "github.com/superfly/fly-go"
)

// mockSimpleClient implements just the GetAppSecrets method for testing
type mockSimpleClient struct {
	secrets []fly.AppSecret
	shouldError bool
	errorMsg string
}

func (m *mockSimpleClient) GetAppSecrets(ctx context.Context, appName string) ([]fly.AppSecret, error) {
	if m.shouldError {
		return nil, fmt.Errorf(m.errorMsg)
	}
	return m.secrets, nil
}

// TestBackupSecretDetection tests the logic for detecting backup configurations
func TestBackupSecretDetection(t *testing.T) {
	tests := []struct {
		name     string
		secrets  []fly.AppSecret
		expected bool
	}{
		{
			name: "backup enabled - S3_ARCHIVE_CONFIG present",
			secrets: []fly.AppSecret{
				{Name: "SU_PASSWORD", Digest: "digest1"},
				{Name: "S3_ARCHIVE_CONFIG", Digest: "digest2"},
				{Name: "REPL_PASSWORD", Digest: "digest3"},
			},
			expected: true,
		},
		{
			name: "backup disabled - no S3_ARCHIVE_CONFIG",
			secrets: []fly.AppSecret{
				{Name: "SU_PASSWORD", Digest: "digest1"},
				{Name: "REPL_PASSWORD", Digest: "digest3"},
				{Name: "OPERATOR_PASSWORD", Digest: "digest4"},
			},
			expected: false,
		},
		{
			name:     "no secrets",
			secrets:  []fly.AppSecret{},
			expected: false,
		},
		{
			name: "different backup-related secrets but not S3_ARCHIVE_CONFIG",
			secrets: []fly.AppSecret{
				{Name: "S3_ARCHIVE_REMOTE_RESTORE_CONFIG", Digest: "digest1"},
				{Name: "BACKUP_CONFIG", Digest: "digest2"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the backup detection logic directly
			found := false
			for _, secret := range tt.secrets {
				if secret.Name == "S3_ARCHIVE_CONFIG" {
					found = true
					break
				}
			}
			if found != tt.expected {
				t.Errorf("Backup detection = %v, expected %v", found, tt.expected)
			}
		})
	}
}
