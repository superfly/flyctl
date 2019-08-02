package docker

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
	"golang.org/x/net/context"
)

func NewDeploymentTag(appName string) string {
	t := time.Now()

	return fmt.Sprintf("registry.fly.io/%s:deployment-%d", appName, t.Unix())
}

type DockerClient struct {
	ctx          context.Context
	docker       *client.Client
	registryAuth string
}

func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewEnvClient()
	if err != nil {
		return nil, err
	}

	accessToken := viper.GetString(flyctl.ConfigAPIAccessToken)

	authConfig := types.AuthConfig{
		Username: accessToken,
		Password: "x",
	}
	encodedJSON, err := json.Marshal(authConfig)
	if err != nil {
		return nil, err
	}
	authStr := base64.URLEncoding.EncodeToString(encodedJSON)

	c := &DockerClient{
		ctx:          context.Background(),
		docker:       cli,
		registryAuth: authStr,
	}

	return c, nil
}

func (c *DockerClient) TagImage(sourceRef, tag string) error {
	return c.docker.ImageTag(c.ctx, sourceRef, tag)
}
