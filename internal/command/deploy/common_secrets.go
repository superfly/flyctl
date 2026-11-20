package deploy

import (
	"regexp"
	"strings"
)

// some common secrets
var commonSecretSubstrings = []string{"KEY", "PRIVATE", "DATABASE_URL", "PASSWORD", "SECRET"}

// Match simple SQLite filenames only. Authorities, URI options and escaped
// characters can change the meaning of a connection string.
var localSQLiteFile = regexp.MustCompile(`^(file:(///|/)?|sqlite3?:/{3,4}|/|\.\.?/)([A-Za-z0-9._-]+/)*[A-Za-z0-9._-]+\.(db|sqlite|sqlite3)$`)

func shouldWarnAboutEnv(name, value string) bool {
	// Allowlist for strings which contain a substring but are not secrets.
	switch name {
	case "DATABASE_URL":
		return !localSQLiteFile.MatchString(value)
	case "AWS_ACCESS_KEY_ID", "TIGRIS_ACCESS_KEY_ID":
		return false
	}

	for _, substr := range commonSecretSubstrings {
		if strings.Contains(name, substr) {
			return true
		}
	}

	return false
}
