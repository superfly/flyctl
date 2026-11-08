package sentry

import (
	"errors"
	"fmt"
	"testing"
)

func TestFingerprintIgnoresIDsAndQuotedValues(t *testing.T) {
	a := fmt.Errorf("failed to acquire lease on machine e7843d1f2d6e08 (request 01JABCD123): timeout")
	b := fmt.Errorf("failed to acquire lease on machine 3d8d9e1b7e4589 (request 01JXYZW987): timeout")
	if got, want := Fingerprint(a), Fingerprint(b); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("same error with different IDs: %v != %v", got, want)
	}

	c := errors.New(`app "my-app" not found at https://api.machines.dev/v1/apps/my-app`)
	d := errors.New(`app "other" not found at https://api.machines.dev/v1/apps/other`)
	if got, want := Fingerprint(c), Fingerprint(d); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("same error with different quoted values: %v != %v", got, want)
	}
}

func TestFingerprintSeparatesDifferentErrors(t *testing.T) {
	limit := errors.New("your organization has reached its machine limit")
	lease := errors.New("failed to acquire lease on machine e7843d1f2d6e08")
	if fmt.Sprint(Fingerprint(limit)) == fmt.Sprint(Fingerprint(lease)) {
		t.Fatal("different errors share a fingerprint")
	}
}

func TestFingerprintIsBounded(t *testing.T) {
	long := errors.New(string(make([]byte, 1000)))
	fp := Fingerprint(long)
	if len(fp) != 2 || len(fp[1]) > fingerprintMaxLen {
		t.Fatalf("fingerprint = %d parts, message %d bytes", len(fp), len(fp[1]))
	}

	if fp := Fingerprint(nil); len(fp) != 1 || fp[0] != "{{ default }}" {
		t.Fatalf("nil error fingerprint = %v", fp)
	}
}
