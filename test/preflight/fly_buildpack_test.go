package preflight

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/superfly/flyctl/test/preflight/testlib"
)

func TestBuildpack(t *testing.T) {
	f := testlib.NewTestEnvFromEnv(t)
	app := f.CreateRandomAppMachines()
	url := fmt.Sprintf("https://%s.fly.dev", app)

	exampleBuildpack := filepath.Join(testlib.RepositoryRoot(), "example-buildpack")

	result := f.Fly("deploy --app %s %s", app, exampleBuildpack)
	t.Log(result.StdOutString())

	var resp *http.Response
	require.Eventually(t, func() bool {
		var err error
		resp, err = http.Get(url)
		return err == nil && resp.StatusCode == http.StatusOK
	}, 20*time.Second, 1*time.Second, "GET %s never returned 200 OK response 20 seconds", url)

	defer resp.Body.Close()
	buf, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "Hello World!", string(buf))
}
