// Package dockerfileurl classifies Dockerfile URLs and makes them safe to display.
package dockerfileurl

import (
	"net/url"
	"strings"
)

func parse(value string) (*url.URL, bool) {
	u, err := url.Parse(value)
	if err != nil || u.Host == "" {
		return nil, false
	}

	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return nil, false
	}

	return u, true
}

// IsURL reports whether value is an HTTP(S) URL.
func IsURL(value string) bool {
	_, ok := parse(value)

	return ok
}

// ForDisplay removes sensitive URL components from HTTP(S) Dockerfile values
// and leaves local paths unchanged.
func ForDisplay(value string) string {
	u, ok := parse(value)
	if !ok {
		return value
	}

	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""

	return u.String()
}
