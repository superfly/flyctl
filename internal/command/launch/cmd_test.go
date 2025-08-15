package launch

import (
	"context"
	"testing"

	"github.com/spf13/pflag"
	"github.com/superfly/flyctl/internal/flag/flagctx"
	"github.com/superfly/flyctl/iostreams"
)

func TestValidatePostgresFlags(t *testing.T) {
	tests := []struct {
		name        string
		dbFlag      string
		noDbFlag    bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid mpg flag",
			dbFlag:      "mpg",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "valid upg flag",
			dbFlag:      "upg",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "valid legacy flag",
			dbFlag:      "legacy",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "valid true flag",
			dbFlag:      "true",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "valid empty flag",
			dbFlag:      "",
			noDbFlag:    false,
			expectError: false,
		},
		{
			name:        "invalid flag value",
			dbFlag:      "invalid",
			noDbFlag:    false,
			expectError: true,
			errorMsg:    "Invalid value 'invalid' for --db flag",
		},
		{
			name:        "conflicting db and no-db",
			dbFlag:      "mpg",
			noDbFlag:    true,
			expectError: true,
			errorMsg:    "Cannot specify both --db and --no-db",
		},
		{
			name:        "conflicting upg and no-db",
			dbFlag:      "upg",
			noDbFlag:    true,
			expectError: true,
			errorMsg:    "Cannot specify both --db and --no-db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a context with iostreams
			ctx := context.Background()
			ctx = iostreams.NewContext(ctx, iostreams.System())

			// Create a test context with flags
			flagSet := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flagSet.String("db", tt.dbFlag, "")
			flagSet.Bool("no-db", tt.noDbFlag, "")
			ctx = flagctx.NewContext(ctx, flagSet)

			err := validatePostgresFlags(ctx)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errorMsg != "" && !containsString(err.Error(), tt.errorMsg) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
