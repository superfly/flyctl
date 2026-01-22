package tigris

import (
	"strings"
	"testing"
)

func TestValidateBucketName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "empty name is valid (will be prompted or auto-generated)",
			input:   "",
			wantErr: false,
		},
		{
			name:      "too short - 1 character",
			input:     "a",
			wantErr:   true,
			errSubstr: "too short",
		},
		{
			name:      "too short - 2 characters",
			input:     "ab",
			wantErr:   true,
			errSubstr: "too short",
		},
		{
			name:    "valid - minimum 3 characters",
			input:   "abc",
			wantErr: false,
		},
		{
			name:    "valid - typical bucket name",
			input:   "my-bucket-123",
			wantErr: false,
		},
		{
			name:    "valid - maximum 63 characters",
			input:   "a23456789012345678901234567890123456789012345678901234567890123",
			wantErr: false,
		},
		{
			name:      "too long - 64 characters (user's original issue)",
			input:     "nexus-staging-chore-new-mechanism-to-translate-city-names-assets",
			wantErr:   true,
			errSubstr: "too long",
		},
		{
			name:      "too long - 100 characters",
			input:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			wantErr:   true,
			errSubstr: "too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBucketName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateBucketName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSubstr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("validateBucketName(%q) error = %v, want error containing %q", tt.input, err, tt.errSubstr)
				}
			}
		})
	}
}
