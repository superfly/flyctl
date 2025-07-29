package deploy

import "strings"

// some common secrets
var commonSecretSubstrings = []string{"KEY", "PRIVATE", "DATABASE_URL", "PASSWORD", "SECRET"}

func containsCommonSecretSubstring(s string) bool {
	// Allowlist for strings which contain a substring but are not secrets.
	switch s {
	case "AWS_ACCESS_KEY_ID", "TIGRIS_ACCESS_KEY_ID":
		return false
	}

	for _, substr := range commonSecretSubstrings {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
