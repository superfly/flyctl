package helpers

import (
	"errors"
	"testing"
)

func TestRetryWhile(t *testing.T) {
	busy := errors.New("busy")
	other := errors.New("boom")
	isBusy := func(err error) bool { return errors.Is(err, busy) }

	tests := []struct {
		name      string
		results   []error // what fn returns on each call; the last repeats
		wantErr   error
		wantCalls int
	}{
		{"success runs once", []error{nil}, nil, 1},
		{"other errors are not retried", []error{other}, other, 1},
		{"stops once it succeeds", []error{busy, busy, nil}, nil, 3},
		{"stops on a different error", []error{busy, other}, other, 2},
		{"gives up after the last attempt", []error{busy}, busy, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			fn := func() error {
				err := tt.results[min(calls, len(tt.results)-1)]
				calls++

				return err
			}

			err := retryWhile(fn, isBusy, 4, 0)
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) || calls != tt.wantCalls {
				t.Fatalf("got %v after %d calls, want %v after %d", err, calls, tt.wantErr, tt.wantCalls)
			}
		})
	}
}

// Off Windows nothing is ever busy, so fn runs exactly once.
func TestRetryIfFileBusyRunsOnceWhenNotBusy(t *testing.T) {
	calls := 0
	if err := RetryIfFileBusy("unused", func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("got %v after %d calls, want nil after 1", err, calls)
	}
}
