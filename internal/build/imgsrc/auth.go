package imgsrc

import (
	"os"

	"github.com/docker/docker/api/types"
)

func authConfigFromToken(token string) map[string]types.AuthConfig {
	authConfigs := map[string]types.AuthConfig{}

	authConfigs["registry.fly.io"] = registryAuth(token)

	dockerhubUsername := os.Getenv("DOCKER_HUB_USERNAME")
	dockerhubPassword := os.Getenv("DOCKER_HUB_PASSWORD")
	if dockerhubUsername != "" && dockerhubPassword != "" {
		cfg := types.AuthConfig{
			Username:      dockerhubUsername,
			Password:      dockerhubPassword,
			ServerAddress: "index.docker.io",
		}
		authConfigs["https://index.docker.io/v1/"] = cfg
	}
	return authConfigs
}
