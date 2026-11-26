package appconfig

import (
	"net"
	"net/url"
	"testing"
)

func TestTemporaryReleaseLookupError(t *testing.T) {
	err := &url.Error{Op: "Post", Err: &net.DNSError{IsTemporary: true}}
	if !transientReleaseError(err) {
		t.Fatal("temporary DNS failure should allow another release lookup")
	}
}
