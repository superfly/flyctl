package tokens

import (
	"testing"
)

func TestAuthorizationHeader(t *testing.T) {
	check := func(macaroonAndUserTokens bool, input, expectedOutput string) {
		t.Helper()
		if tok := Parse(input).normalized(macaroonAndUserTokens, false); tok != expectedOutput {
			t.Fatalf("expected token to be '%s', got '%s'", expectedOutput, tok)
		}
	}

	// scheme stripping
	check(true, "foobar", "foobar")
	check(true, "Bearer foobar", "foobar")
	check(true, "FlyV1 foobar", "foobar")
	check(true, "Bearer FlyV1 foobar", "foobar")
	check(true, "FlyV1 Bearer foobar", "foobar")
	check(true, "BEARER FLYV1 foobar", "foobar")

	// api access token
	check(true, "fm2_foobar,foobar", "fm2_foobar,foobar")
	check(true, "foobar,fm2_foobar", "fm2_foobar,foobar")
	check(true, "foobar", "foobar")
	check(true, "fm2_foobar", "fm2_foobar")

	// non-api access token
	check(false, "fm2_foobar,foobar", "fm2_foobar")
	check(false, "foobar,fm2_foobar", "fm2_foobar")
	check(false, "foobar", "foobar")
	check(false, "fm2_foobar", "fm2_foobar")
}
