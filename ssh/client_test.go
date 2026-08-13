package ssh

import "testing"

func TestSessionTargetValidate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		target  SessionTarget
		wantErr bool
	}{
		{
			// The machine decides where the session lands.
			name: "neither",
		},
		{
			name:   "container",
			target: SessionTarget{Container: "app"},
		},
		{
			name:   "machine",
			target: SessionTarget{Machine: true},
		},
		{
			// Mutually exclusive: rejected rather than silently preferring one.
			name:    "both",
			target:  SessionTarget{Container: "app", Machine: true},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.target.validate()

			if tc.wantErr && err == nil {
				t.Fatal("expected an error, got none")
			}

			if !tc.wantErr && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}
