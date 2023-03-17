package api

import (
	"testing"
)

func TestAuthorizationHeader(t *testing.T) {
	check := func(input, expectedOutput string) {
		t.Helper()
		if hdr := AuthorizationHeader(input); hdr != expectedOutput {
			t.Fatalf("expected header to be '%s', got '%s'", expectedOutput, hdr)
		}
	}

	check("foobar", "Bearer foobar")
	check("FlyV1 foobar", "FlyV1 foobar")
	check("FlyV1foobar", "Bearer FlyV1foobar")
}
