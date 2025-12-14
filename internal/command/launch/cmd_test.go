package launch

import (
	"context"
	"strings"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	fly "github.com/superfly/fly-go"
	"github.com/superfly/flyctl/internal/appconfig"
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
				if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
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

func TestNewDefaultPlanSource(t *testing.T) {
	source := "test source"
	planSource := newDefaultPlanSource(source)

	assert.NotNil(t, planSource)
	assert.Equal(t, source, planSource.appNameSource)
	assert.Equal(t, source, planSource.regionSource)
	assert.Equal(t, source, planSource.orgSource)
	assert.Equal(t, source, planSource.computeSource)
	assert.Equal(t, source, planSource.postgresSource)
	assert.Equal(t, source, planSource.redisSource)
	assert.Equal(t, source, planSource.tigrisSource)
	assert.Equal(t, source, planSource.sentrySource)
}

func TestParseMountOptions(t *testing.T) {
	tests := []struct {
		name          string
		options       string
		expectedMount appconfig.Mount
		expectError   bool
		errMsg        string
	}{
		{
			name:          "empty options",
			options:       "",
			expectedMount: appconfig.Mount{},
		},
		{
			name:    "scheduled_snapshots true",
			options: "scheduled_snapshots=true",
			expectedMount: appconfig.Mount{
				ScheduledSnapshots: fly.Pointer(true),
			},
		},
		{
			name:    "scheduled_snapshots false",
			options: "scheduled_snapshots=false",
			expectedMount: appconfig.Mount{
				ScheduledSnapshots: fly.Pointer(false),
			},
		},
		{
			name:        "scheduled_snapshots invalid value",
			options:     "scheduled_snapshots=invalid",
			expectError: true,
			errMsg:      "invalid value for scheduled_snapshots",
		},
		{
			name:    "snapshot_retention",
			options: "snapshot_retention=7",
			expectedMount: appconfig.Mount{
				SnapshotRetention: fly.Pointer(7),
			},
		},
		{
			name:        "snapshot_retention invalid",
			options:     "snapshot_retention=invalid",
			expectError: true,
			errMsg:      "invalid value for snapshot_retention",
		},
		{
			name:    "initial_size",
			options: "initial_size=10GB",
			expectedMount: appconfig.Mount{
				InitialSize: "10GB",
			},
		},
		{
			name:    "auto_extend_size_threshold",
			options: "auto_extend_size_threshold=80",
			expectedMount: appconfig.Mount{
				AutoExtendSizeThreshold: 80,
			},
		},
		{
			name:        "auto_extend_size_threshold invalid",
			options:     "auto_extend_size_threshold=invalid",
			expectError: true,
			errMsg:      "invalid value for auto_extend_size_threshold",
		},
		{
			name:    "auto_extend_size_increment",
			options: "auto_extend_size_increment=1GB",
			expectedMount: appconfig.Mount{
				AutoExtendSizeIncrement: "1GB",
			},
		},
		{
			name:    "auto_extend_size_limit",
			options: "auto_extend_size_limit=100GB",
			expectedMount: appconfig.Mount{
				AutoExtendSizeLimit: "100GB",
			},
		},
		{
			name:    "multiple options",
			options: "initial_size=10GB,scheduled_snapshots=true,snapshot_retention=14",
			expectedMount: appconfig.Mount{
				InitialSize:        "10GB",
				ScheduledSnapshots: fly.Pointer(true),
				SnapshotRetention:  fly.Pointer(14),
			},
		},
		{
			name:        "unknown option",
			options:     "unknown_option=value",
			expectError: true,
			errMsg:      "unknown mount option",
		},
		{
			name:        "invalid format",
			options:     "invalid_format",
			expectError: true,
			errMsg:      "invalid mount option",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mount := &appconfig.Mount{}
			err := ParseMountOptions(mount, tt.options)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
					return
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error message to contain '%s', got '%s'", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
					return
				}

				assert.Equal(t, tt.expectedMount, *mount)
			}
		})
	}
}
