package ticket

import (
	"strings"
	"testing"
)

func TestValidateTicket(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		priority string
		wantErr  string
	}{
		{"valid low", "my app will not deploy", "low", ""},
		{"valid high", "my app will not deploy", "high", ""},
		{"valid emergency", "my app will not deploy", "emergency", ""},
		{"message too short", "too short", "low", "at least 10 characters"},
		{"message all whitespace", strings.Repeat(" ", 20), "low", "at least 10 characters"},
		{"message too long", strings.Repeat("x", 5001), "low", "at most 5000 characters"},
		{"message at max", strings.Repeat("x", 5000), "low", ""},
		{"message at min", strings.Repeat("x", 10), "low", ""},
		{"invalid priority", "my app will not deploy", "urgent", "invalid priority"},
		{"empty priority", "my app will not deploy", "", "invalid priority"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTicket(tc.message, tc.priority)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
			}
		})
	}
}
