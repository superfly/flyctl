package deploy

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldWarnAboutEnv(t *testing.T) {
	for _, value := range []string{
		"file:/data/sqlite.db", "file:./dev.db", "file:///data/app.sqlite",
		"sqlite3:///data/production.sqlite3", "sqlite3:///litefs/production.sqlite3",
		"sqlite:///app.db", "sqlite:////data/app.db", "/data/app.sqlite3", "./dev.db",
	} {
		assert.False(t, shouldWarnAboutEnv("DATABASE_URL", value), value)
	}
	for _, value := range []string{
		"", "production", "postgres://user:pass@host/app.db", "host=db password=test",
		"file://server/app.db", "sqlite://user:pass@server/app.db", "//server/app.db",
		"sqlite3://host/app.db", "sqlite3:///data/app.db?key=test",
		"sqlite:///user:pass@host/app.db", "sqlite://host/app.db", "/data/extensionless",
		"file:/data/app.db?key=test", "file:/data/app.db#fragment", "file:/data/a%3Fb.db",
	} {
		assert.True(t, shouldWarnAboutEnv("DATABASE_URL", value), value)
	}
	assert.True(t, shouldWarnAboutEnv("THIRDPARTY_SERVICE_SECRET", "file:/data/sqlite.db"))
	assert.True(t, shouldWarnAboutEnv("BACKUP_DATABASE_URL", "file:/data/sqlite.db"))
	assert.False(t, shouldWarnAboutEnv("AWS_ACCESS_KEY_ID", "example"))
}
