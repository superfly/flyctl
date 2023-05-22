package imgsrc

import (
	"os"

	"github.com/docker/docker/api/types"
	"github.com/spf13/viper"
	"github.com/superfly/flyctl/flyctl"
)

func getAPIToken() string {
	// Are either env vars set?
	// check Access token
	accessToken, lookup := os.LookupEnv("FLY_ACCESS_TOKEN")

	if lookup {
		return accessToken
	}

	// check API token
	apiToken, lookup := os.LookupEnv("FLY_API_TOKEN")

	if lookup {
		return apiToken
	}

	viperAuth := viper.GetString(flyctl.ConfigAPIToken)

	return viperAuth
}

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
