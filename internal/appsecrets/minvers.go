package appsecrets

import (
	"context"

	"github.com/pkg/errors"
	"github.com/spf13/viper"

	"github.com/superfly/flyctl/flyctl"
	"github.com/superfly/flyctl/internal/config"
	"github.com/superfly/flyctl/internal/state"
)

func getMinvers() (config.AppSecretsMinvers, error) {
	minvers := config.AppSecretsMinvers{}
	if err := viper.UnmarshalKey(flyctl.ConfigAppSecretsMinvers, &minvers); err != nil {
		return nil, errors.Wrap(err, "invalid application secrets minversions")
	}
	return minvers, nil
}

// GetAppSecretsMinvers returns the minimum secrets version for appName if known or nil.
func GetMinvers(appName string) (*uint64, error) {
	minvers, err := getMinvers()
	if err != nil {
		return nil, err
	}

	if v, ok := minvers[appName]; ok {
		return &v, nil
	}
	return nil, nil
}

func setMinvers(ctx context.Context, appName string, v *uint64) error {
	minvers, err := getMinvers()
	if err != nil {
		return err
	}

	if v == nil {
		delete(minvers, appName)
	} else {
		minvers[appName] = *v
	}

	viper.Set(flyctl.ConfigAppSecretsMinvers, minvers)
	configPath := state.ConfigFile(ctx)
	if err := config.SetAppSecretsMinvers(configPath, minvers); err != nil {
		return errors.Wrap(err, "error saving config file")
	}

	return nil
}

// SetMinvers sets the minimum secrets version for appName and saves it.
func SetMinvers(ctx context.Context, appName string, v uint64) error {
	return setMinvers(ctx, appName, &v)
}

// DeleteMinvers removes the minimum secrets version for appName and saves it.
func DeleteMinvers(ctx context.Context, appName string) error {
	return setMinvers(ctx, appName, nil)
}
