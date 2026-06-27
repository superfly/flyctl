package ssh

import (
	"bytes"
	"testing"
)

func TestWantPTY(t *testing.T) {
	cases := []struct {
		name            string
		allocPTY        bool
		stdinIsTerminal bool
		want            bool
	}{
		{"interactive terminal", true, true, true},
		// Regression test for issue #4536: a caller may ask for a PTY
		// (AllocPTY) while stdin is piped/non-interactive. Allocating a
		// remote PTY in that case makes the terminal line discipline echo
		// the piped bytes back, leaking secrets. We must not request a PTY.
		{"alloc requested but stdin piped", true, false, false},
		{"no alloc, terminal", false, true, false},
		{"no alloc, piped", false, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := wantPTY(tc.allocPTY, tc.stdinIsTerminal); got != tc.want {
				t.Fatalf("wantPTY(%v, %v) = %v, want %v", tc.allocPTY, tc.stdinIsTerminal, got, tc.want)
			}
		})
	}
}

// Piped, non-terminal stdin (the scenario in issue #4536) must never be
// detected as a terminal, so wantPTY collapses to false for it.
func TestGetFdNonTerminal(t *testing.T) {
	if _, ok := getFd(bytes.NewBufferString("TEST=123\n")); ok {
		t.Fatal("getFd reported a bytes.Buffer as a terminal")
	}
}
