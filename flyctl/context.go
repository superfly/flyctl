package flyctl

import "os"

func CurrentAppName() string {
	appName := os.Getenv("FLY_APP")
	if appName != "" {
		return appName
	}

	if manifest, err := LoadManifest(DefaultManifestPath()); err == nil {
		appName = manifest.AppName
	}

	return appName
}
