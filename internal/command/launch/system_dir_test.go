package launch

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/internal/flyerr"
)

func TestCheckNotInSystemDir(t *testing.T) {
	for _, tc := range []struct {
		dir     string
		wantErr bool
	}{
		{dir: `C:\WINDOWS\system32`, wantErr: true},
		{dir: `C:\Windows\System32`, wantErr: true},
		{dir: `c:/windows/system32/`, wantErr: true},
		{dir: `C:\Windows`, wantErr: true},
		{dir: `C:\Users\me\code\my-app`},
		{dir: `C:\WindowsApps\my-app`},
		{dir: `D:\Windows\system32`},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			err := checkNotInSystemDir(tc.dir, `C:\Windows`)

			if !tc.wantErr {
				assert.NoError(t, err)

				return
			}
			assert.EqualError(t, err, "fly launch can't run in "+tc.dir)
			assert.Contains(t, flyerr.GetErrorSuggestion(err), "cd into your project")
		})
	}

	assert.NoError(t, checkNotInSystemDir(`C:\Windows\System32`, ""), "no SystemRoot, nothing to check")
}
