package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/superfly/flyctl/internal/command/root"
)

// stdoutJSONMode lets Cobra resolve the command and parse its flags, so every
// spelling Cobra accepts is decided the way the command will see it.
func TestStdoutJSONMode(t *testing.T) {
	t.Setenv("FLY_CONFIG_DIR", t.TempDir())
	for _, tc := range []struct {
		args, env string
		want      bool
	}{
		{"mpg create --json", "", true},
		{"mpg create -j", "", true},
		{"mpg --verbose create -j", "", true},
		{"mpg create -jn my-db", "", true},
		{"mpg create -nj", "", false}, // --name=j
		{"mpg create --org --json", "", false},
		{"mpg create --json=false", "1", false},
		{"mpg create --json=true", "", true},
		{"mpg create --json=maybe", "", true},
		{"mpg create", "1", true},
		{"mpg create", "", false},
		// A flag error without JSON intent stays in text mode.
		{"mpg create --name x --region iad --definitely-invalid", "", false},
		{"mpg fork src --not-a-flag", "", false},
		{"mpg create --volume-size=nope", "", false},
		{"mpg create --definitely-invalid", "1", true},
		{"mpg create --definitely-invalid --json=false", "1", false},
		{"mpg create --definitely-invalid --json", "", true},
		{"mpg create --json --definitely-invalid", "", true},
		{"mpg create --volume-size=nope -j", "", true},
		{"mpg create --definitely-invalid -jn x", "", true},
		{"mpg create --definitely-invalid -nj", "", false},
		{"mpg create --definitely-invalid --org --json", "", false},
		{"mpg create --definitely-invalid --json=maybe", "", true},
		{"mpg create --definitely-invalid -- --json", "", false},
		{"mpg create --definitely-invalid -j=false", "", false},
		{"mpg create --help", "", false},
		{"mpg create --help --json", "", true},
		{"mpg create -- --json", "", false},
		{"mpg fork src --json", "", true},
		{"mpg restore src -j", "", true},
		{"mpg user create cid -ju app", "", true},
		{"mpg users create cid --json", "", true},
		{"mpg dbs create cid --json", "", true},
		{"mpg db create cid -jn app", "", true},
		{"mpg database create cid", "true", true},
		{"mpg databases create cid --json", "", true},
		{"mpg list --json", "", false},
		{"mpg users list cid --json", "", false},
		{"apps create --json", "1", false},
		{"mpg nonexistent --json", "", false},
	} {
		t.Run(tc.args+"/"+tc.env, func(t *testing.T) {
			t.Setenv("FLY_JSON", tc.env)
			assert.Equal(t, tc.want, stdoutJSONMode(root.New(), strings.Fields(tc.args)))
		})
	}
}
