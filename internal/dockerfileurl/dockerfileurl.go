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

// LooksLikeURL reports whether value starts with an HTTP(S) scheme, including
// malformed values that should fail closed instead of being treated as paths.
func LooksLikeURL(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))

	return strings.HasPrefix(value, "http:") || strings.HasPrefix(value, "https:")
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
		if LooksLikeURL(value) {
			return "invalid URL"
		}

		return value
	}

	return redact(u)
}

// ForRequestError removes sensitive components from an absolute or relative
// URL reference returned by net/http.
func ForRequestError(value string) string {
	u, err := url.Parse(value)
	if err != nil {
		return "invalid URL"
	}

	return redact(u)
}

func redact(u *url.URL) string {
	u.User = nil
	u.RawQuery = ""
	u.ForceQuery = false
	u.Fragment = ""

	return u.String()
}
