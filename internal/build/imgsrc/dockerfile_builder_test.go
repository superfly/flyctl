package imgsrc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/pkg/iostreams"
)

func TestBuildDockerfileApp(t *testing.T) {
	t.Skip()
	df := NewDockerClientFactory(DockerDaemonTypeLocal, nil, "test-app")

	dfStrategy := dockerfileStrategy{}
	testStreams, _, _, _ := iostreams.Test()

	wd, err := os.Getwd()
	assert.NoError(t, err)

	workingDir := filepath.Join(wd, "testdata", "dockerfile_app")
	configFilePath := filepath.Join(workingDir, "fly.toml")

	appConfig, err := flyctl.LoadAppConfig(configFilePath)
	assert.NoError(t, err)

	opts := ImageOptions{
		AppName:    "test-app",
		WorkingDir: workingDir,
		AppConfig:  appConfig,
		Tag:        "test-dockerfile-app",
	}

	img, err := dfStrategy.Run(context.TODO(), df, testStreams, opts)
	fmt.Printf("err: %#v %T", err, err)
	assert.NoError(t, err)
	assert.NotNil(t, img)

	assert.Equal(t, "test-dockerfile-app", img.Tag)
}
